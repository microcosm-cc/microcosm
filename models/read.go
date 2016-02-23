package models

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/lib/pq"

	h "github.com/microcosm-cc/microcosm/helpers"
)

// ReadType describes when an item was last read by a profile
type ReadType struct {
	ID         int64
	ProfileID  int64
	ItemTypeID int64
	ItemID     int64
	Read       time.Time
}

// ReadScopeType describes the item to be considered read
type ReadScopeType struct {
	ItemID     int64  `json:"itemId"`
	ItemType   string `json:"itemType"`
	ItemTypeID int64
}

// GetLastReadTime fetches the last time an item has been read by a profile
func GetLastReadTime(
	itemTypeID int64,
	itemID int64,
	profileID int64,
) (
	time.Time,
	int,
	error,
) {

	var lastRead time.Time

	if profileID == 0 {
		return lastRead, http.StatusOK, nil
	}

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return lastRead, http.StatusInternalServerError, err
	}

	rows, err := db.Query(`
SELECT MAX(read)
  FROM read
 WHERE (
           (item_type_id = $1 AND item_id = $2) 
        OR (item_type_id = 2 AND item_id = (
             SELECT microcosm_id 
               FROM flags
              WHERE item_type_id = $1
                AND item_id = $2
               )
           )
       )
   AND profile_id = $3`,
		itemTypeID,
		itemID,
		profileID,
	)
	if err != nil {
		glog.Errorf(
			"db.Query(%d, %d, %d) %+v",
			itemTypeID,
			itemID,
			profileID,
			err,
		)
		return lastRead, http.StatusInternalServerError,
			fmt.Errorf("Database query failed")
	}
	defer rows.Close()

	var read pq.NullTime
	for rows.Next() {
		err = rows.Scan(&read)
		if err != nil {
			glog.Errorf("rows.Scan() %+v", err)
			return lastRead, http.StatusInternalServerError,
				fmt.Errorf("Row parsing error")
		}
	}
	err = rows.Err()
	if err != nil {
		glog.Errorf("rows.Err() %+v", err)
		return lastRead, http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows")
	}
	rows.Close()

	if read.Valid {
		lastRead = read.Time
	}

	return lastRead, http.StatusOK, nil
}

// MarkAllHuddlesForAllProfilesAsReadOnSite is used by the importer
func MarkAllHuddlesForAllProfilesAsReadOnSite(siteID int64) error {
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Error(err)
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
DELETE FROM read
 WHERE read_id IN (
SELECT r.read_id
  FROM read r
       JOIN profiles p ON p.profile_id = r.profile_id
 WHERE p.site_id = $1
   AND r.item_type_id = 5
)`,
		siteID,
	)
	if err != nil {
		glog.Error(err)
		return err
	}

	_, err = tx.Exec(`
INSERT INTO read (
       item_type_id
      ,item_id
      ,profile_id
      ,read
)
SELECT 5 AS item_type_id
      ,0 AS item_id
      ,p.profile_id
      ,NOW() AS read
  FROM profiles p
 WHERE site_id = $1`,
		siteID,
	)
	if err != nil {
		glog.Error(err)
		return err
	}

	_, err = tx.Exec(`
UPDATE profiles
   SET unread_huddles = 0
 WHERE site_id = $1`,
		siteID,
	)
	if err != nil {
		glog.Error(err)
		return err
	}

	err = tx.Commit()
	if err != nil {
		glog.Error(err)
		return err
	}

	return nil
}

// MarkAllMicrocosmsForAllProfilesAsReadOnSite is used by the importer
func MarkAllMicrocosmsForAllProfilesAsReadOnSite(siteID int64) error {
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Error(err)
		return err
	}
	defer tx.Rollback()

	// Delete all conversation, poll, event markers
	_, err = tx.Exec(`
DELETE
  FROM read r
 WHERE read_id IN (
           SELECT r.read_id
             FROM read r
             JOIN profiles p ON p.site_id = $1
                            AND p.profile_id = r.profile_id
            WHERE item_type_id IN (2,6,7,9)
       )`,
		siteID,
	)
	if err != nil {
		glog.Error(err)
		return err
	}

	_, err = tx.Exec(`
INSERT INTO read (
       item_type_id
      ,item_id
      ,profile_id
      ,read
)
SELECT 2 AS item_type_id
      ,m.microcosm_id AS item_id
      ,p.profile_id
      ,NOW() AS read
  FROM profiles p
  JOIN microcosms m ON m.site_id = p.site_id
 WHERE p.site_id = $1`,
		siteID,
	)
	if err != nil {
		glog.Error(err)
		return err
	}

	err = tx.Commit()
	if err != nil {
		glog.Error(err)
		return err
	}

	return nil
}

// MarkAsRead will record when an item has been read by a profile
func MarkAsRead(
	itemTypeID int64,
	itemID int64,
	profileID int64,
	updateTime time.Time,
) (
	int,
	error,
) {
	// Some validation
	if profileID == 0 {
		glog.Infof("profileId == 0")
		return http.StatusOK, nil
	}

	if (itemTypeID != h.ItemTypes[h.ItemTypeUpdate] && itemID == 0) ||
		itemTypeID == 0 {

		glog.Errorln(
			"(itemTypeId != h.ItemTypes[h.ItemTypeUpdate] && itemId == 0) || " +
				"itemTypeId == 0",
		)
		return http.StatusExpectationFailed,
			fmt.Errorf("itemTypeId and/or itemId was null when MarkAsRead " +
				"was called. This is illogical, you need to tell us what " +
				"is being marked as read.")
	}

	if updateTime.IsZero() {
		glog.Errorln("updateTime.IsZero()")
		return http.StatusExpectationFailed,
			fmt.Errorf("MarkAsRead has been called but the time supplied " +
				"is null. You need to tell us when the item was read.")
	}

	// Do the deed
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("h.GetTransaction() %+v", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Could not start transaction")
	}
	defer tx.Rollback()

	reads := []ReadType{}
	initial := ReadType{
		ItemTypeID: itemTypeID,
		ItemID:     itemID,
		ProfileID:  profileID,
		Read:       updateTime,
	}
	reads = append(reads, initial)

	// Mark all children as read too
	if itemTypeID == h.ItemTypes[h.ItemTypeMicrocosm] {
		rows, err := tx.Query(`--GetChildMicrocosms
SELECT microcosm_id
  FROM microcosms
 WHERE path <@ (SELECT path FROM microcosms WHERE microcosm_id = $1)
   AND microcosm_id != $1`,
			initial.ItemID,
		)
		if err != nil {
			glog.Errorf("tx.Query(%d) %+v", initial.ItemID, err)
			return http.StatusInternalServerError,
				fmt.Errorf("Fetch of child microcosms failed")
		}
		defer rows.Close()

		for rows.Next() {
			read := ReadType{
				ItemTypeID: initial.ItemTypeID,
				ProfileID:  initial.ProfileID,
				Read:       initial.Read,
			}
			err = rows.Scan(
				&read.ItemID,
			)
			if err != nil {
				glog.Errorf("Scan failed %+v", err)
				return http.StatusInternalServerError,
					fmt.Errorf("Scan failed")
			}

			reads = append(reads, read)
		}
		err = rows.Err()
		if err != nil {
			glog.Errorf("Error fetching rows %+v", err)
			return http.StatusInternalServerError,
				fmt.Errorf("Error fetching rows")
		}
		rows.Close()
	}

	for _, m := range reads {
		status, err := m.upsert(tx)
		if err != nil {
			glog.Errorf("m.upsert(tx) %+v", err)
			return status, err
		}
	}

	switch itemTypeID {
	case h.ItemTypes[h.ItemTypeSite]:
		// Site has been marked read, which means all Microcosms and items are
		// implicitly read. So we should delete those records, but not the rows
		// referring to huddles or the site itself.

		_, err = tx.Exec(`
DELETE FROM read
 WHERE item_type_id NOT IN (1, 5) -- 1 = site, 5 = huddle
   AND profile_id = $1`,
			initial.ProfileID,
		)
		if err != nil {
			glog.Errorf("tx.Exec(%d) %+v", initial.ProfileID, err)
			return http.StatusInternalServerError,
				fmt.Errorf("Deletion of read items failed")
		}
	case h.ItemTypes[h.ItemTypeMicrocosm]:
		it := []string{}
		for _, read := range reads {
			it = append(it, strconv.FormatInt(read.ItemID, 10))
		}
		microcosmIDs := `{` + strings.Join(it, `,`) + `}`

		// Microcosm has been marked read, so delete all item level rows that
		// belong to this Microcosm as those are implicitly read as of now
		_, err = tx.Exec(`
DELETE FROM read
 WHERE read_id IN (
           SELECT read_id 
            FROM (
                     SELECT item_id
                           ,item_type_id 
                       FROM flags
                      WHERE microcosm_id = ANY ($1::bigint[])
                        AND parent_item_id IS NULL
                        AND item_type_id != 2
                 ) i
            JOIN read ON read.item_id = i.item_id
                     AND read.item_type_id = i.item_type_id
                     AND read.profile_id = $2
       )
   AND profile_id = $2`,
			microcosmIDs,
			initial.ProfileID,
		)
		if err != nil {
			glog.Errorf("tx.Exec(%d, %d) %+v", initial.ItemID, initial.ProfileID, err)
			return http.StatusInternalServerError,
				fmt.Errorf("Deletion of read items failed")
		}
	case h.ItemTypes[h.ItemTypeHuddle]:
		if itemID == 0 {
			// All huddles have been marked read, so we should delete the
			// individual records for older microcosms *and* set the unread
			// huddle count as 0 on the profile.
			_, err = tx.Exec(`
DELETE FROM read
 WHERE item_type_id = 5 -- 5 = huddle
   AND item_id <> 0
   AND profile_id = $1`,
				initial.ProfileID,
			)
			if err != nil {
				glog.Errorf("tx.Exec(%d) %+v", initial.ProfileID, err)
				return http.StatusInternalServerError,
					fmt.Errorf("Deletion of read items failed")
			}

			updateUnreadHuddleCount(tx, profileID)
		}
	}

	err = tx.Commit()
	if err != nil {
		glog.Errorf("tx.Commit() %+v", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed")
	}

	return http.StatusOK, nil
}

// upsert inserts or updates the last read time
func (m *ReadType) upsert(tx *sql.Tx) (int, error) {
	res, err := tx.Exec(`
UPDATE read
   SET read = GREATEST(read, $4)
 WHERE item_type_id = $1
   AND item_id = $2
   AND profile_id = $3`,
		m.ItemTypeID,
		m.ItemID,
		m.ProfileID,
		m.Read,
	)
	if err != nil {
		glog.Errorf(
			"tx.Exec(%d, %d, %d, %v) %+v",
			m.ItemTypeID,
			m.ItemID,
			m.ProfileID,
			m.Read,
			err,
		)
		return http.StatusInternalServerError,
			fmt.Errorf("Error executing update")
	}

	// If update did not create any rows then we need to insert
	if rowsAffected, _ := res.RowsAffected(); rowsAffected == 0 {
		// Note that even though we just proved that it didn't exist, we could
		// have multiple transactions doing this and we'are playing safe and
		// defensively by doing a NOT EXISTS check, this shouldn't fail at all.
		glog.Infof("INSERT (%d, %d)", m.ItemID, m.ProfileID)
		res, err = tx.Exec(`
INSERT INTO read
    (item_type_id, item_id, profile_id, read)
SELECT $1, $2, $3, $4
 WHERE NOT EXISTS (
           SELECT read_id
             FROM read
            WHERE item_type_id = $1
              AND item_id = $2
              AND profile_id = $3
       )`,
			m.ItemTypeID,
			m.ItemID,
			m.ProfileID,
			m.Read,
		)
		if err != nil {
			glog.Errorf(
				"tx.Exec(%d, %d, %d, %v) %+v",
				m.ItemTypeID,
				m.ItemID,
				m.ProfileID,
				m.Read,
				err,
			)
			tx.Rollback()
			return http.StatusInternalServerError,
				fmt.Errorf("Error inserting data")
		}
	}

	return http.StatusOK, nil
}

// MarkScopeAsRead will mark something like the site or a microcosm and all of
// it's contents as read
func MarkScopeAsRead(profileID int64, rs ReadScopeType) (int, error) {
	if rs.ItemTypeID == h.ItemTypes[h.ItemTypeSite] {
		return MarkAllAsRead(profileID)
	} else if rs.ItemTypeID == h.ItemTypes[h.ItemTypeHuddle] && rs.ItemID >= 0 {
		// It's fine to mark huddleId = 0 as read, this is equivalent to marking
		// all huddles as read
		return MarkAsRead(rs.ItemTypeID, rs.ItemID, profileID, time.Now())
	} else if rs.ItemTypeID > 0 && rs.ItemID > 0 {
		return MarkAsRead(rs.ItemTypeID, rs.ItemID, profileID, time.Now())
	}

	return http.StatusBadRequest,
		fmt.Errorf(
			"ItemTypeId and ItemId must be specified to mark an item read",
		)
}

// MarkAllAsRead marks everything on the site as read except huddles
func MarkAllAsRead(profileID int64) (int, error) {
	// This method lies... we mark everything except huddles as read
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("h.GetTransaction() %+v", err)
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
DELETE FROM read
 WHERE profile_id = $1
   AND item_type_id IN (2, 6, 9, 10, 16)`,
		profileID,
	)
	if err != nil {
		glog.Errorf("tx.Exec(%d) %+v", profileID, err)
		tx.Rollback()
		return http.StatusInternalServerError, err
	}

	_, err = tx.Exec(`
INSERT INTO READ
       (profile_id, item_type_id, item_id, "read")
SELECT p.profile_id
      ,2 AS item_type_id
      ,m.microcosm_id AS item_id
      ,NOW() AS "read"
  FROM profiles p
      ,microcosms m
 WHERE p.profile_id = $1
   AND m.site_id = p.site_id`,
		profileID,
	)
	if err != nil {
		glog.Errorf("stmt2.Exec(%d) %+v", profileID, err)
		tx.Rollback()
		return http.StatusInternalServerError, err
	}

	err = tx.Commit()
	if err != nil {
		glog.Errorf("tx.Commit() %+v", err)
		tx.Rollback()
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

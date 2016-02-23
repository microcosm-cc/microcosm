package models

import (
	"github.com/golang/glog"

	c "github.com/microcosm-cc/microcosm/cache"
	h "github.com/microcosm-cc/microcosm/helpers"
)

// DeleteOrphanedHuddles finds huddles that no longer have participants and
// deletes them
func DeleteOrphanedHuddles() {
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Error(err)
		return
	}
	defer tx.Rollback()

	// Identify orphaned huddles
	rows, err := tx.Query(`--DeleteOrphanedHuddles
SELECT h.huddle_id
  FROM huddles h
  LEFT JOIN huddle_profiles hp ON h.huddle_id = hp.huddle_id
 WHERE hp.huddle_id IS NULL`)
	if err != nil {
		glog.Error(err)
		return
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var huddleID int64
		err = rows.Scan(&huddleID)
		if err != nil {
			glog.Error(err)
			return
		}
		ids = append(ids, huddleID)
	}
	err = rows.Err()
	if err != nil {
		glog.Error(err)
		return
	}
	rows.Close()

	if len(ids) == 0 {
		return
	}

	revisionsStmt, err := tx.Prepare(`--DeleteOrphanedHuddles
DELETE
  FROM revisions
 WHERE comment_id IN (
       SELECT comment_id
         FROM comments
        WHERE item_type_id = 5
          AND item_id = $1`)
	if err != nil {
		glog.Error(err)
		return
	}

	commentsStmt, err := tx.Prepare(`--DeleteOrphanedHuddles
DELETE
  FROM comments
 WHERE item_type_id = 5
   AND item_id = $1`)
	if err != nil {
		glog.Error(err)
		return
	}

	huddleStmt, err := tx.Prepare(`--DeleteOrphanedHuddles
DELETE
  FROM huddles
 WHERE huddle_id = $1`)
	if err != nil {
		glog.Error(err)
		return
	}

	for _, huddleID := range ids {
		// delete comment + revisions that belong to this huddle
		// May well be best to expand the above SQL rather than execute lots
		// of single delete commands.

		_, err = revisionsStmt.Exec(huddleID)
		if err != nil {
			glog.Error(err)
			return
		}

		_, err = commentsStmt.Exec(huddleID)
		if err != nil {
			glog.Error(err)
			return
		}

		_, err = huddleStmt.Exec(huddleID)
		if err != nil {
			glog.Error(err)
			return
		}

	}

	tx.Commit()
}

// UpdateAllSiteStats updates the site stats across all sites
func UpdateAllSiteStats() {
	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	rows, err := db.Query(
		`SELECT site_id FROM sites WHERE is_deleted IS NOT TRUE`,
	)
	if err != nil {
		glog.Error(err)
		return
	}
	defer rows.Close()

	// For each site, fetch stats and purge cache.
	ids := []int64{}
	for rows.Next() {

		var siteID int64
		err = rows.Scan(&siteID)
		if err != nil {
			glog.Error(err)
			return
		}

		ids = append(ids, siteID)
	}
	err = rows.Err()
	if err != nil {
		glog.Error(err)
		return
	}
	rows.Close()

	for _, siteID := range ids {
		err = UpdateSiteStats(siteID)
		if err != nil {
			glog.Error(err)
			return
		}
	}
}

// UpdateMetricsCron updates the metrics used by the internal dashboard by the
// admins. This includes counts of the number of items, changes in active
// sites, etc.
func UpdateMetricsCron() {
	UpdateMetrics()
}

// UpdateMicrocosmItemCounts updates the count of items for microcosms, which is
// used to order the microcosms
//
// This is pure housekeeping, the numbers are maintained through increments and
// decrements as stuff is added and deleted, but there are edge cases that may
// result in the numbers not being accurate (batch deletions, things being
// deleted via PATCH, etc).
//
// This function is designed to calculate the real numbers and only update rows
// where the numbers are not the real numbers.
func UpdateMicrocosmItemCounts() {
	// No transaction as we don't care for accuracy on these updates
	//
	// Note: This function doesn't even return errors, we don't even care
	// if the occasional UPDATE fails. All this effects are the ordering of
	// Microcosms on a page... this is fairly non-critical as it seldom changes
	// in established sites
	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	// Update item and comment counts
	_, err = db.Exec(
		`UPDATE microcosms m
   SET comment_count = s.comment_count
      ,item_count = s.item_count
  FROM (
           SELECT dm.microcosm_id
                 ,COALESCE(SUM(counts.item_count), 0) AS item_count
                 ,COALESCE(SUM(counts.comment_count), 0) AS comment_count
             FROM microcosms dm
             LEFT JOIN (
                      -- Calculate item counts
                      SELECT microcosm_id
                            ,COUNT(*) AS item_count
                            ,0 AS comment_count
                        FROM flags
                       WHERE item_type_id IN (6,7,9)
                         AND microcosm_is_deleted IS NOT TRUE
                         AND microcosm_is_moderated IS NOT TRUE
                         AND parent_is_deleted IS NOT TRUE
                         AND parent_is_moderated IS NOT TRUE
                         AND item_is_deleted IS NOT TRUE
                         AND item_is_moderated IS NOT TRUE
                       GROUP BY microcosm_id
                       UNION
                      -- Calculate comment counts
                      SELECT microcosm_id
                            ,0 AS item_count
                            ,COUNT(*) AS comment_count
                        FROM flags
                       WHERE item_type_id = 4
                         AND parent_item_type_id IN (6,7,9)
                         AND microcosm_is_deleted IS NOT TRUE
                         AND microcosm_is_moderated IS NOT TRUE
                         AND parent_is_deleted IS NOT TRUE
                         AND parent_is_moderated IS NOT TRUE
                         AND item_is_deleted IS NOT TRUE
                         AND item_is_moderated IS NOT TRUE
                       GROUP BY microcosm_id
                  ) counts ON dm.microcosm_id = counts.microcosm_id
            GROUP BY dm.microcosm_id
       ) s
 WHERE m.microcosm_id = s.microcosm_id
   AND (
           m.item_count <> s.item_count
        OR m.comment_count <> s.comment_count
       )`)
	if err != nil {
		glog.Error(err)
		return
	}

	db.Exec(`--updateMicrocosmSequence
UPDATE microcosms m
   SET sequence = ms.sequence
  FROM (
           SELECT microcosm_id
                 ,row_number() OVER(
                      partition BY site_id
                      ORDER BY count DESC, microcosm_id
                  ) AS sequence
             FROM (
                      SELECT m.site_id
                            ,m.microcosm_id
                            ,COALESCE(
                                 (SELECT SUM(comment_count) + SUM(item_count)
                                    FROM microcosms
                                   WHERE path <@ m.path
                                     AND is_deleted IS NOT TRUE
                                     AND is_moderated IS NOT TRUE
                                 ),
                                 0
                             ) AS count
                        FROM microcosms m
                       GROUP BY m.site_id, m.microcosm_id
                       ORDER BY site_id, count DESC
                  ) AS mm
       ) ms
  WHERE m.microcosm_id = ms.microcosm_id`)
	if err != nil {
		glog.Error(err)
		return
	}
}

// UpdateProfileCounts updates the count of profiles per site
func UpdateProfileCounts() {
	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	rows, err := db.Query(
		`SELECT site_id FROM sites WHERE is_deleted IS NOT TRUE`,
	)
	if err != nil {
		glog.Error(err)
		return
	}
	defer rows.Close()

	// For each site, fetch stats and purge cache.
	ids := []int64{}
	for rows.Next() {

		var siteID int64
		err = rows.Scan(&siteID)
		if err != nil {
			glog.Error(err)
			return
		}

		ids = append(ids, siteID)
	}
	err = rows.Err()
	if err != nil {
		glog.Error(err)
		return
	}
	rows.Close()

	for _, siteID := range ids {
		_, err = UpdateCommentCountForAllProfiles(siteID)
		if err != nil {
			glog.Error(err)
			return
		}
	}
}

// UpdateViewCounts reads from the views table and will SUM the number of views
// and update all of the associated conversations and events with the new view
// count.
func UpdateViewCounts() {
	// No transaction as we don't care for accuracy on these updates
	//
	// Note: This function doesn't even return errors, we don't even care
	// if the occasional UPDATE fails.
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Error(err)
		return
	}
	defer tx.Rollback()

	type View struct {
		ItemTypeID int64
		ItemID     int64
	}

	rows, err := tx.Query(`--UpdateViewCounts
SELECT item_type_id
      ,item_id
  FROM views
 GROUP BY item_type_id, item_id`)
	if err != nil {
		glog.Error(err)
		return
	}
	defer rows.Close()

	var (
		views               []View
		updateConversations bool
		updateEvents        bool
		updatePolls         bool
		updateQuestions     bool
	)
	for rows.Next() {
		var view View
		err = rows.Scan(
			&view.ItemTypeID,
			&view.ItemID,
		)
		if err != nil {
			glog.Error(err)
			return
		}

		switch view.ItemTypeID {
		case h.ItemTypes[h.ItemTypeConversation]:
			updateConversations = true
		case h.ItemTypes[h.ItemTypeEvent]:
			updateEvents = true
		case h.ItemTypes[h.ItemTypePoll]:
			updatePolls = true
		case h.ItemTypes[h.ItemTypeQuestion]:
			updateQuestions = true
		}

		views = append(views, view)
	}
	err = rows.Err()
	if err != nil {
		glog.Error(err)
		return
	}
	rows.Close()

	if len(views) == 0 {
		// No views to update
		return
	}

	// Our updates are a series of updates in the database, we don't even
	// read the records as why intervene like that?

	// Update conversations
	if updateConversations {
		_, err = tx.Exec(`--UpdateViewCounts
UPDATE conversations c
   SET view_count = view_count + v.views
  FROM (
        SELECT item_id
              ,COUNT(*) AS views
          FROM views
         WHERE item_type_id = 6
         GROUP BY item_id
       ) AS v
 WHERE c.conversation_id = v.item_id`)
		if err != nil {
			glog.Error(err)
			return
		}
	}

	// Update events
	if updateEvents {
		_, err = tx.Exec(`--UpdateViewCounts
UPDATE events e
   SET view_count = view_count + v.views
  FROM (
        SELECT item_id
              ,COUNT(*) AS views
          FROM views
         WHERE item_type_id = 9
         GROUP BY item_id
       ) AS v
 WHERE e.event_id = v.item_id`)
		if err != nil {
			glog.Error(err)
			return
		}
	}

	// Update polls
	if updatePolls {
		_, err = tx.Exec(`--UpdateViewCounts
UPDATE polls p
   SET view_count = view_count + v.views
  FROM (
        SELECT item_id
              ,COUNT(*) AS views
          FROM views
         WHERE item_type_id = 7
         GROUP BY item_id
       ) AS v
 WHERE p.poll_id = v.item_id;`)
		if err != nil {
			glog.Error(err)
			return
		}
	}

	// Update questions
	if updateQuestions {
		_, err = tx.Exec(`--UpdateViewCounts
UPDATE questions q
   SET view_count = view_count + v.views
  FROM (
        SELECT item_id
              ,COUNT(*) AS views
          FROM views
         WHERE item_type_id = 10
         GROUP BY item_id
       ) AS v
 WHERE q.question_id = v.item_id;`)
		if err != nil {
			glog.Error(err)
			return
		}
	}

	// Clear views, and the quickest way to do that is just truncate the table
	_, err = tx.Exec(`TRUNCATE TABLE views`)
	if err != nil {
		glog.Error(err)
		return
	}

	tx.Commit()

	for _, view := range views {
		PurgeCacheByScope(c.CacheItem, view.ItemTypeID, view.ItemID)
	}

	return
}

// UpdateWhosOnline updates the site_stats with the current number of people
// online on a site
func UpdateWhosOnline() {
	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	// Update item and comment counts
	_, err = db.Exec(`--UpdateWhosOnline
UPDATE site_stats s
   SET online_profiles = online
  FROM (
           SELECT site_id
                 ,COUNT(*) AS online
             FROM profiles
            WHERE last_active > NOW() - interval '90 minute'
            GROUP BY site_id
       ) p
 WHERE p.site_id = s.site_id`)
	if err != nil {
		glog.Error(err)
		return
	}

	// Purge the stats cache
	rows, err := db.Query(
		`SELECT site_id FROM sites WHERE is_deleted IS NOT TRUE`,
	)
	if err != nil {
		glog.Error(err)
		return
	}
	defer rows.Close()

	// For each site, fetch stats and purge cache.
	ids := []int64{}
	for rows.Next() {
		var siteID int64
		err = rows.Scan(&siteID)
		if err != nil {
			glog.Error(err)
			return
		}

		ids = append(ids, siteID)
	}
	err = rows.Err()
	if err != nil {
		glog.Error(err)
		return
	}
	rows.Close()

	for _, siteID := range ids {
		go PurgeCacheByScope(c.CacheCounts, h.ItemTypes[h.ItemTypeSite], siteID)
	}
}

// DeleteOldUpdates purges old updates from the updates table
func DeleteOldUpdates() {
	db, err := h.GetConnection()
	if err != nil {
		glog.Error(err)
		return
	}

	// Update knowledge of parent items
	_, err = db.Exec(`--UpdateUpdatesParent
UPDATE updates u
   SET parent_item_type_id = f.parent_item_type_id
      ,parent_item_id = f.parent_item_id
  FROM flags f 
 WHERE f.item_type_id = 4
   AND u.parent_item_type_id = 0
   AND u.parent_item_id = 0
   AND u.item_type_id = f.item_type_id
   AND u.item_id = f.item_id;`)
	if err != nil {
		glog.Error(err)
		return
	}

	// Remove all non-MAX items for a parent and for_profile pair
	_, err = db.Exec(`TRUNCATE updates_latest;`)
	if err != nil {
		glog.Error(err)
		return
	}

	// Remove all non-MAX items for a parent and for_profile pair
	_, err = db.Exec(`--InsertUpdatesLatest
INSERT INTO updates_latest
SELECT MAX(update_id) update_id
  FROM updates
 WHERE update_type_id IN (1,4)
 GROUP BY for_profile_id
      ,parent_item_type_id
      ,parent_item_id;
`)
	if err != nil {
		glog.Error(err)
		return
	}

	// Remove all non-MAX items for a parent and for_profile pair
	_, err = db.Exec(`--PurgeOldUpdates
DELETE FROM updates u
 WHERE update_type_id IN (1,4)
   AND for_profile_id != 0
   AND parent_item_type_id != 0
   AND parent_item_id != 0
   AND NOT EXISTS (
           SELECT update_id
             FROM updates_latest l
            WHERE l.update_id = u.update_id
       );
`)
	if err != nil {
		glog.Error(err)
		return
	}

	_, err = db.Exec(`VACUUM FULL ANALYZE updates;`)
	if err != nil {
		glog.Error(err)
		return
	}
}

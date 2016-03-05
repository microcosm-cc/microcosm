package models

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"

	h "github.com/microcosm-cc/microcosm/helpers"
)

type YMGType struct {
	ID            int64     `json:"-"`
	ItemTypeID    int64     `json:"-"`
	ItemID        int64     `json:"-"`
	ProfileID     int64     `json:"-"`
	Created       time.Time `json:"-"`
	ItemProfileID int64     `json:"-"`
	Value         int       `json:"-"`
	Yay           bool      `json:"yay"`
	Meh           bool      `json:"meh"`
	Grr           bool      `json:"grr"`
}

func GetYMG(
	itemTypeID int64,
	itemID int64,
	profileID int64,
) (YMGType, int, error) {

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return YMGType{}, http.StatusInternalServerError, err
	}

	sqlQuery := `--GetYMG
SELECT ymg_id
      ,item_type_id
      ,item_id
      ,profile_id
      ,created
      ,item_profile_id
      ,value
  FROM ymg
 WHERE item_type_id = $1
   AND item_id = $2
   AND profile_id = $3`

	var ymg YMGType
	err = db.QueryRow(
		sqlQuery,
		itemTypeID,
		itemID,
		profileID,
	).Scan(
		&ymg.ID,
		&ymg.ItemTypeID,
		&ymg.ItemID,
		&ymg.ProfileID,
		&ymg.Created,
		&ymg.ItemProfileID,
		&ymg.Value,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return YMGType{}, http.StatusNotFound, fmt.Errorf("not found")
		}
		return YMGType{}, http.StatusInternalServerError, err
	}

	switch ymg.Value {
	case 1:
		ymg.Yay = true
	case 0:
		ymg.Meh = true
	case -1:
		ymg.Grr = true
	default:
		return YMGType{}, http.StatusInternalServerError,
			fmt.Errorf("value must be one of 1|0|-1")
	}

	return ymg, http.StatusOK, nil
}

func (m *YMGType) Update() (int, error) {
	if (m.Yay && m.Meh) || (m.Yay && m.Grr) || (m.Meh && m.Grr) {
		return http.StatusInternalServerError,
			fmt.Errorf("only one of yay, meh or grr can be true")
	}

	switch {
	case m.Yay:
		m.Value = 1
	case m.Meh:
		m.Value = 0
	case m.Grr:
		m.Value = -1
	default:
		return http.StatusInternalServerError,
			fmt.Errorf("one of yay, meh or grr must be true")
	}

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return http.StatusInternalServerError, err
	}

	if m.ID != 0 {
		db.Exec(`--UpdateYMG`,
			m.ID,
			m.Value,
		)
		return http.StatusOK, nil
	}

	err = db.QueryRow(`--InsertYMG
INSERT INTO ymg(
    item_type_id, item_id, profile_id, created, item_profile_id,
    value
) VALUES (
    $1, $2, $3, (SELECT created_by FROM flags WHERE item_type_id = $1 and item_id = $2), $4, 
    $5
) RETURNING ymg_id`,
		m.ItemTypeID,
		m.ItemID,
		m.ProfileID,
		m.Created,
		m.Value,
	).Scan(
		&m.ID,
	)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func (m *YMGType) Delete() (int, error) {
	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return http.StatusInternalServerError, err
	}

	_, err = db.Exec(`--DeleteYMG
DELETE FROM ymg WHERE ymg_id = $1
`, m.ID)

	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

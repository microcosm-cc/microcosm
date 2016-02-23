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

	c "github.com/microcosm-cc/microcosm/cache"
	h "github.com/microcosm-cc/microcosm/helpers"
)

// QuestionsType is a collection of questions
type QuestionsType struct {
	Questions h.ArrayType    `json:"questions"`
	Meta      h.CoreMetaType `json:"meta"`
}

// QuestionSummaryType is a summary of a question
type QuestionSummaryType struct {
	ItemSummary
	ItemSummaryMeta

	AcceptedAnswerID int64 `json:"acceptedAnswer,omitempty"`
}

// QuestionType is a question
type QuestionType struct {
	ItemDetail
	ItemDetailCommentsAndMeta

	AcceptedAnswerID int64 `json:"acceptedAnswer,omitempty"`
}

// Validate returns true if a question is valid
func (m *QuestionType) Validate(
	siteID int64,
	profileID int64,
	exists bool,
	isImport bool,
) (
	int,
	error,
) {
	preventShouting := true
	m.Title = CleanSentence(m.Title, preventShouting)

	if strings.Trim(m.Title, " ") == "" {
		return http.StatusBadRequest, fmt.Errorf("Title is a required field")
	}

	if !exists {
		// Does the Microcosm specified exist on this site?
		_, status, err := GetMicrocosmSummary(
			siteID,
			m.MicrocosmID,
			profileID,
		)
		if err != nil {
			return status, err
		}
	}

	if exists && !isImport {
		if m.ID < 1 {
			return http.StatusBadRequest, fmt.Errorf(
				"The supplied ID ('%d') cannot be zero or negative",
				m.ID,
			)
		}

		if strings.Trim(m.Meta.EditReason, " ") == "" ||
			len(m.Meta.EditReason) == 0 {

			return http.StatusBadRequest,
				fmt.Errorf("You must provide a reason for the update")

		}

		m.Meta.EditReason = CleanSentence(m.Meta.EditReason, preventShouting)
	}

	if m.MicrocosmID <= 0 {
		return http.StatusBadRequest,
			fmt.Errorf("You must specify a Microcosm ID")
	}

	m.Meta.Flags.SetVisible()

	return http.StatusOK, nil
}

// Hydrate populates a partially populated struct
func (m *QuestionType) Hydrate(siteID int64) (int, error) {

	profile, status, err := GetProfileSummary(siteID, m.Meta.CreatedByID)
	if err != nil {
		return status, err
	}
	m.Meta.CreatedBy = profile

	if m.Meta.EditedByNullable.Valid {
		profile, status, err :=
			GetProfileSummary(siteID, m.Meta.EditedByNullable.Int64)
		if err != nil {
			return status, err
		}
		m.Meta.EditedBy = profile
	}

	if status, err := m.FetchBreadcrumb(); err != nil {
		return status, err
	}

	return http.StatusOK, nil
}

// Hydrate populates a partially populated struct
func (m *QuestionSummaryType) Hydrate(
	siteID int64,
) (
	int,
	error,
) {
	profile, status, err := GetProfileSummary(siteID, m.Meta.CreatedByID)
	if err != nil {
		return status, err
	}
	m.Meta.CreatedBy = profile

	switch m.LastComment.(type) {
	case LastComment:
		lastComment := m.LastComment.(LastComment)

		profile, status, err =
			GetProfileSummary(siteID, lastComment.CreatedByID)
		if err != nil {
			return status, err
		}

		lastComment.CreatedBy = profile
		m.LastComment = lastComment
	}

	if status, err := m.FetchBreadcrumb(); err != nil {
		return status, err
	}

	return http.StatusOK, nil
}

// Insert saves a question
func (m *QuestionType) Insert(siteID int64, profileID int64) (int, error) {
	status, err := m.Validate(siteID, profileID, false, false)
	if err != nil {
		return status, err
	}

	dupeKey := "dupe_" + h.MD5Sum(
		strconv.FormatInt(m.MicrocosmID, 10)+
			m.Title+
			strconv.FormatInt(m.Meta.CreatedByID, 10),
	)
	v, ok := c.GetInt64(dupeKey)
	if ok {
		m.ID = v
		return http.StatusOK, nil
	}

	status, err = m.insert(siteID, profileID)
	if status == http.StatusOK {
		// 5 minute dupe check
		c.SetInt64(dupeKey, m.ID, 60*5)
	}

	return status, err
}

// Import saves a question with duplicate checking
func (m *QuestionType) Import(siteID int64, profileID int64) (int, error) {
	status, err := m.Validate(siteID, profileID, true, true)
	if err != nil {
		return status, err
	}

	return m.insert(siteID, profileID)
}

func (m *QuestionType) insert(siteID int64, profileID int64) (int, error) {
	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	var insertID int64
	err = tx.QueryRow(`--Create Question
INSERT INTO questions (
    microcosm_id, title, created, created_by, view_count,
    is_deleted, is_moderated, is_open, is_sticky, accepted_answer_id
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
) RETURNING question_id`,
		m.MicrocosmID,
		m.Title,
		m.Meta.Created,
		m.Meta.CreatedByID,
		m.ViewCount,

		m.Meta.Flags.Deleted,
		m.Meta.Flags.Moderated,
		m.Meta.Flags.Open,
		m.Meta.Flags.Sticky,
		m.AcceptedAnswerID,
	).Scan(
		&insertID,
	)
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf(
				"Error inserting data and returning ID: %v",
				err.Error(),
			)
	}

	m.ID = insertID

	err = IncrementMicrocosmItemCount(tx, m.MicrocosmID)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	err = tx.Commit()
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	PurgeCache(h.ItemTypes[h.ItemTypeQuestion], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// Update updates a question
func (m *QuestionType) Update(siteID int64, profileID int64) (int, error) {

	status, err := m.Validate(siteID, profileID, true, false)
	if err != nil {
		return status, err
	}

	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`--Update Question
UPDATE questions
   SET microcosm_id = $2,
       title = $3,
       edited = $4,
       edited_by = $5,
       edit_reason = $6
 WHERE question_id = $1`,
		m.ID,
		m.MicrocosmID,
		m.Title,
		m.Meta.EditedNullable,
		m.Meta.EditedByNullable,
		m.Meta.EditReason,
	)
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Update failed: %v", err.Error())
	}

	err = tx.Commit()
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	PurgeCache(h.ItemTypes[h.ItemTypeQuestion], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// Patch partially updates a saved question
func (m *QuestionType) Patch(
	ac AuthContext,
	patches []h.PatchType,
) (
	int,
	error,
) {
	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	for _, patch := range patches {

		m.Meta.EditedNullable = pq.NullTime{Time: time.Now(), Valid: true}
		m.Meta.EditedByNullable = sql.NullInt64{Int64: ac.ProfileID, Valid: true}

		var column string
		patch.ScanRawValue()
		switch patch.Path {
		case "/meta/flags/sticky":
			column = "is_sticky"
			m.Meta.Flags.Sticky = patch.Bool.Bool
			m.Meta.EditReason =
				fmt.Sprintf("Set sticky to %t", m.Meta.Flags.Sticky)
		case "/meta/flags/open":
			column = "is_open"
			m.Meta.Flags.Open = patch.Bool.Bool
			m.Meta.EditReason =
				fmt.Sprintf("Set open to %t", m.Meta.Flags.Open)
		case "/meta/flags/deleted":
			column = "is_deleted"
			m.Meta.Flags.Deleted = patch.Bool.Bool
			m.Meta.EditReason =
				fmt.Sprintf("Set delete to %t", m.Meta.Flags.Deleted)
		case "/meta/flags/moderated":
			column = "is_moderated"
			m.Meta.Flags.Moderated = patch.Bool.Bool
			m.Meta.EditReason =
				fmt.Sprintf("Set moderated to %t", m.Meta.Flags.Moderated)
		default:
			return http.StatusBadRequest,
				fmt.Errorf("Unsupported path in patch replace operation")
		}

		m.Meta.Flags.SetVisible()

		_, err = tx.Exec(`--Update Question Flags
UPDATE questions
   SET `+column+` = $2
      ,is_visible = $3
      ,edited = $4
      ,edited_by = $5
      ,edit_reason = $6
 WHERE question_id = $1`,
			m.ID,
			patch.Bool.Bool,
			m.Meta.Flags.Visible,
			m.Meta.EditedNullable,
			m.Meta.EditedByNullable,
			m.Meta.EditReason,
		)
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("Update failed: %v", err.Error())
		}
	}

	err = tx.Commit()
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	PurgeCache(h.ItemTypes[h.ItemTypeQuestion], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// Delete deletes a question
func (m *QuestionType) Delete() (int, error) {
	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`--Delete Question
UPDATE questions
   SET is_deleted = true
      ,is_visible = false
 WHERE question_id = $1`,
		m.ID,
	)
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Delete failed: %v", err.Error())
	}

	err = DecrementMicrocosmItemCount(tx, m.MicrocosmID)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	err = tx.Commit()
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	PurgeCache(h.ItemTypes[h.ItemTypeQuestion], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// GetQuestion fetches a question
func GetQuestion(
	siteID int64,
	id int64,
	profileID int64,
) (
	QuestionType,
	int,
	error,
) {
	if id == 0 {
		return QuestionType{}, http.StatusNotFound,
			fmt.Errorf("Question not found")
	}

	// Get from cache if it's available
	mcKey := fmt.Sprintf(mcQuestionKeys[c.CacheDetail], id)
	if val, ok := c.Get(mcKey, QuestionType{}); ok {
		m := val.(QuestionType)

		m.Hydrate(siteID)

		return m, http.StatusOK, nil
	}

	// Retrieve resource
	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return QuestionType{}, http.StatusInternalServerError, err
	}

	// TODO(buro9): admins and mods could see this with isDeleted=true in the
	// querystring
	var m QuestionType

	err = db.QueryRow(`--GetQuestion
SELECT q.question_id
      ,q.microcosm_id
      ,q.title
      ,q.created
      ,q.created_by

      ,q.edited
      ,q.edited_by
      ,q.edit_reason
      ,q.is_sticky
      ,q.is_open
      
      ,q.is_deleted
      ,q.is_moderated
      ,q.is_visible
      ,q.accepted_answer_id
  FROM questions q
       JOIN flags f ON f.site_id = $2
                   AND f.item_type_id = 10
                   AND f.item_id = q.question_id
 WHERE q.question_id = $1
   AND is_deleted(10, q.question_id) IS FALSE`,
		id,
		siteID,
	).Scan(
		&m.ID,
		&m.MicrocosmID,
		&m.Title,
		&m.Meta.Created,
		&m.Meta.CreatedByID,

		&m.Meta.EditedNullable,
		&m.Meta.EditedByNullable,
		&m.Meta.EditReasonNullable,
		&m.Meta.Flags.Sticky,
		&m.Meta.Flags.Open,

		&m.Meta.Flags.Deleted,
		&m.Meta.Flags.Moderated,
		&m.Meta.Flags.Visible,
		&m.AcceptedAnswerID,
	)
	if err == sql.ErrNoRows {
		glog.Warningf("Question not found for id %d", id)
		return QuestionType{}, http.StatusNotFound,
			fmt.Errorf("Resource with ID %d not found", id)

	} else if err != nil {
		glog.Errorf("db.Query(%d) %+v", id, err)
		return QuestionType{}, http.StatusInternalServerError,
			fmt.Errorf("Database query failed")
	}

	if m.Meta.EditReasonNullable.Valid {
		m.Meta.EditReason = m.Meta.EditReasonNullable.String
	}

	if m.Meta.EditedNullable.Valid {
		m.Meta.Edited =
			m.Meta.EditedNullable.Time.Format(time.RFC3339Nano)
	}

	m.Meta.Links =
		[]h.LinkType{
			h.GetLink("self", "", h.ItemTypeQuestion, m.ID),
			h.GetLink(
				"microcosm",
				GetMicrocosmTitle(m.MicrocosmID),
				h.ItemTypeMicrocosm,
				m.MicrocosmID,
			),
		}

	// Update cache
	c.Set(mcKey, m, mcTTL)

	m.Hydrate(siteID)
	return m, http.StatusOK, nil
}

// GetQuestionSummary fetches a summary of a question
func GetQuestionSummary(
	siteID int64,
	id int64,
	profileID int64,
) (
	QuestionSummaryType,
	int,
	error,
) {
	// Get from cache if it's available
	mcKey := fmt.Sprintf(mcQuestionKeys[c.CacheSummary], id)
	if val, ok := c.Get(mcKey, QuestionSummaryType{}); ok {
		m := val.(QuestionSummaryType)
		m.Hydrate(siteID)
		return m, http.StatusOK, nil
	}

	// Retrieve resource
	db, err := h.GetConnection()
	if err != nil {
		return QuestionSummaryType{}, http.StatusInternalServerError, err
	}

	// TODO(buro9): admins and mods could see this with isDeleted=true in the
	// querystring
	var m QuestionSummaryType
	err = db.QueryRow(`--GetQuestionSummary
SELECT question_id
      ,microcosm_id
      ,title
      ,created
      ,created_by
      ,is_sticky
      ,is_open
      ,is_deleted
      ,is_moderated
      ,is_visible
      ,(SELECT COUNT(*) AS total_comments
          FROM flags
         WHERE parent_item_type_id = 10
           AND parent_item_id = $1
           AND microcosm_is_deleted IS NOT TRUE
           AND microcosm_is_moderated IS NOT TRUE
           AND parent_is_deleted IS NOT TRUE
           AND parent_is_moderated IS NOT TRUE
           AND item_is_deleted IS NOT TRUE
           AND item_is_moderated IS NOT TRUE) AS comment_count
      ,view_count
      ,accepted_answer_id
  FROM questions
 WHERE question_id = $1
   AND is_deleted(10, $1) IS FALSE`,
		id,
	).Scan(
		&m.ID,
		&m.MicrocosmID,
		&m.Title,
		&m.Meta.Created,
		&m.Meta.CreatedByID,
		&m.Meta.Flags.Sticky,
		&m.Meta.Flags.Open,
		&m.Meta.Flags.Deleted,
		&m.Meta.Flags.Moderated,
		&m.Meta.Flags.Visible,
		&m.CommentCount,
		&m.ViewCount,
		&m.AcceptedAnswerID,
	)
	if err == sql.ErrNoRows {
		return QuestionSummaryType{}, http.StatusNotFound,
			fmt.Errorf("Resource with ID %d not found", id)

	} else if err != nil {
		return QuestionSummaryType{}, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}

	lastComment, status, err :=
		GetLastComment(h.ItemTypes[h.ItemTypeQuestion], m.ID)
	if err != nil {
		return QuestionSummaryType{}, status,
			fmt.Errorf("Error fetching last comment: %v", err.Error())
	}

	if lastComment.Valid {
		m.LastComment = lastComment
	}

	m.Meta.Links =
		[]h.LinkType{
			h.GetLink("self", "", h.ItemTypeQuestion, m.ID),
			h.GetLink(
				"microcosm",
				GetMicrocosmTitle(m.MicrocosmID),
				h.ItemTypeMicrocosm,
				m.MicrocosmID,
			),
		}

	// Update cache
	c.Set(mcKey, m, mcTTL)

	m.Hydrate(siteID)
	return m, http.StatusOK, nil
}

// GetQuestions returns a collection of questions
func GetQuestions(
	siteID int64,
	profileID int64,
	limit int64,
	offset int64,
) (
	[]QuestionSummaryType,
	int64,
	int64,
	int,
	error,
) {
	// Retrieve resources
	db, err := h.GetConnection()
	if err != nil {
		return []QuestionSummaryType{}, 0, 0,
			http.StatusInternalServerError, err
	}

	rows, err := db.Query(`--GetQuestions
WITH m AS (
    SELECT m.microcosm_id
      FROM microcosms m
      LEFT JOIN ignores_expanded i ON i.profile_id = $3
                                  AND i.item_type_id = 2
                                  AND i.item_id = m.microcosm_id
     WHERE i.profile_id IS NULL
       AND (get_effective_permissions(m.site_id, m.microcosm_id, 2, m.microcosm_id, $3)).can_read IS TRUE
)
SELECT COUNT(*) OVER() AS total
      ,f.item_id
  FROM flags f
  LEFT JOIN ignores i ON i.profile_id = $3
                     AND i.item_type_id = f.item_type_id
                     AND i.item_id = f.item_id
 WHERE f.site_id = $1
   AND i.profile_id IS NULL
   AND f.item_type_id = $2
   AND f.microcosm_is_deleted IS NOT TRUE
   AND f.microcosm_is_moderated IS NOT TRUE
   AND f.parent_is_deleted IS NOT TRUE
   AND f.parent_is_moderated IS NOT TRUE
   AND f.item_is_deleted IS NOT TRUE
   AND f.item_is_moderated IS NOT TRUE
   AND f.microcosm_id IN (SELECT * FROM m)
 ORDER BY f.item_is_sticky DESC
         ,f.last_modified DESC
 LIMIT $4
OFFSET $5`,
		siteID,
		h.ItemTypes[h.ItemTypeQuestion],
		profileID,
		limit,
		offset,
	)
	if err != nil {
		return []QuestionSummaryType{}, 0, 0,
			http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}
	defer rows.Close()

	var ems []QuestionSummaryType

	var total int64
	for rows.Next() {
		var id int64
		err = rows.Scan(
			&total,
			&id,
		)
		if err != nil {
			return []QuestionSummaryType{}, 0, 0,
				http.StatusInternalServerError,
				fmt.Errorf("Row parsing error: %v", err.Error())
		}

		m, status, err := GetQuestionSummary(siteID, id, profileID)
		if err != nil {
			return []QuestionSummaryType{}, 0, 0, status, err
		}

		ems = append(ems, m)
	}
	err = rows.Err()
	if err != nil {
		return []QuestionSummaryType{}, 0, 0,
			http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows: %v", err.Error())
	}
	rows.Close()

	pages := h.GetPageCount(total, limit)
	maxOffset := h.GetMaxOffset(total, limit)

	if offset > maxOffset {
		return []QuestionSummaryType{}, 0, 0,
			http.StatusBadRequest, fmt.Errorf(
				"not enough records, "+
					"offset (%d) would return an empty page",
				offset,
			)
	}

	return ems, total, pages, http.StatusOK, nil
}

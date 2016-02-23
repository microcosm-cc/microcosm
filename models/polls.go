package models

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	c "github.com/microcosm-cc/microcosm/cache"
	h "github.com/microcosm-cc/microcosm/helpers"
)

// PollsType is an array of polls
type PollsType struct {
	Polls h.ArrayType    `json:"polls"`
	Meta  h.CoreMetaType `json:"meta"`
}

// PollSummaryType is a summary of a poll
type PollSummaryType struct {
	ItemSummary

	PollQuestion       string      `json:"question"`
	Multi              bool        `json:"multi"`
	PollOpen           bool        `json:"pollOpen"`
	VotingEndsNullable pq.NullTime `json:"-"`
	VotingEnds         string      `json:"pollCloses,omitempty"`

	ItemSummaryMeta
}

// PollType is a poll
type PollType struct {
	ItemDetail

	// Type Specific
	PollQuestion string           `json:"question"`
	Multi        bool             `json:"multi"`
	PollOpen     bool             `json:"pollOpen"`
	Choices      []PollChoiceType `json:"choices,omitempty"`
	VoterCount   int64            `json:"voterCount"`

	// Type Specific Optional
	VotingEndsNullable pq.NullTime `json:"-"`
	VotingEnds         string      `json:"pollCloses,omitempty"`

	ItemDetailCommentsAndMeta
}

// PollChoiceType is a single choice on a poll
type PollChoiceType struct {
	ID         int64  `json:"id"`
	Choice     string `json:"choice"`
	Order      int64  `json:"order"`
	Votes      int64  `json:"votes"`
	VoterCount int64  `json:"voterCount"`
}

// Validate returns true if the poll is valid
func (m *PollType) Validate(
	siteID int64,
	profileID int64,
	exists bool,
) (
	int,
	error,
) {

	preventShouting := true
	m.Title = CleanSentence(m.Title, preventShouting)
	m.PollQuestion = CleanSentence(m.PollQuestion, preventShouting)

	// Does the Microcosm specified exist on this site?
	if !exists {
		_, status, err := GetMicrocosmSummary(
			siteID,
			m.MicrocosmID,
			profileID,
		)
		if err != nil {
			return status, err
		}
	}

	if exists {
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

	if strings.Trim(m.Title, " ") == "" {
		return http.StatusBadRequest, fmt.Errorf("Title is a required field")
	}

	if strings.Trim(m.PollQuestion, " ") == "" {
		return http.StatusBadRequest,
			fmt.Errorf("You must supply a question that the poll will answer")
	}

	if m.Choices == nil || len(m.Choices) == 0 {
		return http.StatusBadRequest,
			fmt.Errorf("You must supply choices for the poll")
	}

	for ii := 0; ii < len(m.Choices); ii++ {
		m.Choices[ii].Choice = CleanSentence(m.Choices[ii].Choice, preventShouting)

		if strings.Trim(m.Choices[ii].Choice, " ") == "" {
			return http.StatusBadRequest,
				fmt.Errorf("Your poll choices must be populated")
		}
	}

	if strings.Trim(m.VotingEnds, " ") != "" {
		votingEnds, err := time.Parse(time.RFC3339, m.VotingEnds)
		if err != nil {
			return http.StatusBadRequest, err
		}

		if !exists && votingEnds.Unix() < time.Now().Unix() {
			return http.StatusBadRequest,
				fmt.Errorf("Voting cannot close in the past")
		}

		m.VotingEndsNullable = pq.NullTime{Time: votingEnds, Valid: true}
	}

	// Set defaults on child nodes
	for ii := 0; ii < len(m.Choices); ii++ {

		// Order is a sequence, we set the order to be the order that it was
		// posted to us.
		m.Choices[ii].Order = int64(ii) + 1
	}

	m.Meta.Flags.SetVisible()

	return http.StatusOK, nil
}

// Hydrate populates a partially populated struct
func (m *PollType) Hydrate(siteID int64) (int, error) {
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
func (m *PollSummaryType) Hydrate(siteID int64) (int, error) {
	profile, status, err := GetProfileSummary(siteID, m.Meta.CreatedByID)
	if err != nil {
		return status, err
	}
	m.Meta.CreatedBy = profile

	if status, err := m.FetchBreadcrumb(); err != nil {
		return status, err
	}

	return http.StatusOK, nil
}

// Insert saves the poll to the database
func (m *PollType) Insert(siteID int64, profileID int64) (int, error) {

	status, err := m.Validate(siteID, profileID, false)
	if err != nil {
		return status, err
	}

	// Inputs are good, save to the database
	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
INSERT INTO polls (
    microcosm_id, title, question, created, created_by,
    voting_ends, is_poll_open, is_multiple_choice
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8
) RETURNING poll_id`,
		m.MicrocosmID,
		m.Title,
		m.PollQuestion,
		m.Meta.Created,
		m.Meta.CreatedByID,
		m.VotingEndsNullable,
		m.PollOpen,
		m.Multi,
	)

	var insertID int64
	err = row.Scan(&insertID)
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Error inserting data and returning ID: %v", err.Error())
	}
	m.ID = insertID

	for ii := 0; ii < len(m.Choices); ii++ {
		_, err := tx.Exec(`
INSERT INTO choices (
       poll_id, title, sequence
) VALUES (
       $1, $2, $3
)`,
			insertID,
			m.Choices[ii].Choice,
			m.Choices[ii].Order,
		)
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("Insert failed: %v", err.Error())
		}
	}

	err = IncrementMicrocosmItemCount(tx, m.MicrocosmID)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	err = tx.Commit()
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	PurgeCache(h.ItemTypes[h.ItemTypePoll], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// Update saves changes to a poll
func (m *PollType) Update(siteID int64, profileID int64) (int, error) {

	status, err := m.Validate(siteID, profileID, true)
	if err != nil {
		return status, err
	}

	// Update resource
	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	choiceIDs := make([]string, len(m.Choices))
	seen := 0
	for ii := 0; ii < len(m.Choices); ii++ {
		if m.Choices[ii].ID > 0 {
			choiceIDs[seen] = strconv.FormatInt(m.Choices[ii].ID, 10)
			seen++
		}
	}
	choiceIDs = choiceIDs[0:seen]

	if len(choiceIDs) > 0 {

		_, err = tx.Exec(fmt.Sprintf(`
DELETE FROM choices
 WHERE poll_id = $1
   AND choice_id NOT IN (%s)`,
			strings.Join(choiceIDs[0:seen], ","),
		),
			m.ID,
		)
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("Delete of choices failed: %v", err.Error())
		}
	}

	for ii := 0; ii < len(m.Choices); ii++ {
		if m.Choices[ii].ID > 0 {
			_, err = tx.Exec(`
UPDATE choices
   SET title = $1,
       sequence = $2,
       vote_count = $3
 WHERE choice_id = $4`,
				m.Choices[ii].Choice,
				m.Choices[ii].Order,
				m.Choices[ii].Votes,
				m.Choices[ii].ID,
			)
		} else {
			_, err = tx.Exec(`
INSERT INTO choices (
    poll_id, title, sequence
) VALUES (
    $1, $2, $3
)`,
				m.ID,
				m.Choices[ii].Choice,
				m.Choices[ii].Order,
			)
		}

		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("Insert or Update of choices failed: %v", err.Error())
		}
	}

	_, err = tx.Exec(`
UPDATE polls
   SET microcosm_id = $2
      ,title = $3
      ,question = $4
      ,edited = $5
      ,edited_by = $6
      ,edit_reason = $7
      ,voting_ends = $8
      ,is_poll_open = $9
      ,is_multiple_choice = $10
 WHERE poll_id = $1`,
		m.ID,
		m.MicrocosmID,
		m.Title,
		m.PollQuestion,
		m.Meta.EditedNullable,
		m.Meta.EditedByNullable,
		m.Meta.EditReason,
		m.VotingEndsNullable,
		m.PollOpen,
		m.Multi,
	)
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Update of poll failed: %v", err.Error())
	}

	err = tx.Commit()
	if err != nil {
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	PurgeCache(h.ItemTypes[h.ItemTypePoll], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// Patch allows for partial updates to the poll
func (m *PollType) Patch(ac AuthContext, patches []h.PatchType) (int, error) {

	// Update resource
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
		_, err = tx.Exec(`
UPDATE polls
   SET `+column+` = $2
      ,is_visible = $3
      ,edited = $4
      ,edited_by = $5
      ,edit_reason = $6
 WHERE poll_id = $1`,
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

	PurgeCache(h.ItemTypes[h.ItemTypePoll], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// Delete removes a poll from the database
func (m *PollType) Delete() (int, error) {

	// Delete resource
	tx, err := h.GetTransaction()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
UPDATE polls
   SET is_deleted = true
      ,is_visible = false
 WHERE poll_id = $1`,
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

	PurgeCache(h.ItemTypes[h.ItemTypePoll], m.ID)
	PurgeCache(h.ItemTypes[h.ItemTypeMicrocosm], m.MicrocosmID)

	return http.StatusOK, nil
}

// GetPoll fetches a poll
func GetPoll(siteID int64, id int64, profileID int64) (PollType, int, error) {

	// Get from cache if it's available
	mcKey := fmt.Sprintf(mcPollKeys[c.CacheDetail], id)
	if val, ok := c.Get(mcKey, PollType{}); ok {
		m := val.(PollType)
		m.Hydrate(siteID)
		return m, http.StatusOK, nil
	}

	// Retrieve resource
	db, err := h.GetConnection()
	if err != nil {
		return PollType{}, http.StatusInternalServerError, err
	}

	// TODO(buro9): admins and mods could see this with isDeleted=true in the querystring
	var m PollType
	err = db.QueryRow(`
SELECT p.poll_id
      ,p.microcosm_id
      ,p.title
      ,p.question
      ,p.created
      ,p.created_by
      ,p.edited
      ,p.edited_by
      ,p.edit_reason
      ,p.voter_count
      ,p.voting_ends
      ,p.is_sticky
      ,p.is_open
      ,p.is_deleted
      ,p.is_moderated
      ,p.is_visible
      ,p.is_poll_open
      ,p.is_multiple_choice
  FROM polls p
      ,microcosms m
 WHERE p.microcosm_id = m.microcosm_id
   AND m.site_id = $1
   AND m.is_deleted IS NOT TRUE
   AND m.is_moderated IS NOT TRUE
   AND p.poll_id = $2
   AND p.is_deleted IS NOT TRUE
   AND p.is_moderated IS NOT TRUE`,
		siteID,
		id,
	).Scan(
		&m.ID,
		&m.MicrocosmID,
		&m.Title,
		&m.PollQuestion,
		&m.Meta.Created,
		&m.Meta.CreatedByID,
		&m.Meta.EditedNullable,
		&m.Meta.EditedByNullable,
		&m.Meta.EditReasonNullable,
		&m.VoterCount,
		&m.VotingEndsNullable,
		&m.Meta.Flags.Sticky,
		&m.Meta.Flags.Open,
		&m.Meta.Flags.Deleted,
		&m.Meta.Flags.Moderated,
		&m.Meta.Flags.Visible,
		&m.PollOpen,
		&m.Multi,
	)
	if err == sql.ErrNoRows {
		return PollType{}, http.StatusNotFound,
			fmt.Errorf("Resource with ID %d not found", id)

	} else if err != nil {
		return PollType{}, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}

	if m.Meta.EditReasonNullable.Valid {
		m.Meta.EditReason = m.Meta.EditReasonNullable.String
	}
	if m.Meta.EditedNullable.Valid {
		m.Meta.Edited = m.Meta.EditedNullable.Time.Format(time.RFC3339Nano)
	}
	if m.VotingEndsNullable.Valid {
		m.VotingEnds = m.VotingEndsNullable.Time.Format(time.RFC3339Nano)
	}

	rows2, err := db.Query(`
SELECT choice_id,
       title,
       vote_count,
       voter_count,
       sequence
  FROM choices
 WHERE poll_id = $1
 ORDER BY sequence ASC`,
		m.ID,
	)
	if err != nil {
		return PollType{}, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}
	defer rows2.Close()

	var choices []PollChoiceType

	for rows2.Next() {
		choice := PollChoiceType{}
		err = rows2.Scan(
			&choice.ID,
			&choice.Choice,
			&choice.Votes,
			&choice.VoterCount,
			&choice.Order,
		)
		choices = append(choices, choice)
		if err != nil {
			return PollType{}, http.StatusInternalServerError,
				fmt.Errorf("Row parsing error: %v", err.Error())
		}
	}
	err = rows2.Err()
	if err != nil {
		return PollType{}, http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows2: %v", err.Error())
	}
	rows2.Close()

	m.Choices = choices
	m.Meta.Links =
		[]h.LinkType{
			h.GetLink("self", "", h.ItemTypePoll, m.ID),
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

// GetPollSummary fetches a summary of a poll
func GetPollSummary(
	siteID int64,
	id int64,
	profileID int64,
) (
	PollSummaryType,
	int,
	error,
) {

	// Get from cache if it's available
	mcKey := fmt.Sprintf(mcPollKeys[c.CacheSummary], id)
	if val, ok := c.Get(mcKey, PollSummaryType{}); ok {
		m := val.(PollSummaryType)
		_, status, err := GetMicrocosmSummary(
			siteID,
			m.MicrocosmID,
			profileID,
		)
		if err != nil {
			return PollSummaryType{}, status, err
		}
		m.Hydrate(siteID)
		return m, 0, nil
	}

	// Retrieve resource
	db, err := h.GetConnection()
	if err != nil {
		return PollSummaryType{}, http.StatusInternalServerError, err
	}

	// TODO(buro9): admins and mods could see this with isDeleted=true in the querystring
	var m PollSummaryType
	err = db.QueryRow(`
SELECT poll_id
      ,microcosm_id
      ,title
      ,question
      ,created
      ,created_by
      ,voting_ends
      ,is_sticky
      ,is_open
      ,is_deleted
      ,is_moderated
      ,is_visible
      ,is_poll_open
      ,is_multiple_choice
      ,(SELECT COUNT(*) AS total_comments
          FROM flags
         WHERE parent_item_type_id = 7
           AND parent_item_id = $1
           AND item_is_deleted IS NOT TRUE
           AND item_is_moderated IS NOT TRUE) AS comment_count
      ,view_count
  FROM polls
 WHERE poll_id = $1
   AND is_deleted(7, poll_id) IS FALSE`,
		id,
	).Scan(
		&m.ID,
		&m.MicrocosmID,
		&m.Title,
		&m.PollQuestion,
		&m.Meta.Created,
		&m.Meta.CreatedByID,
		&m.VotingEndsNullable,
		&m.Meta.Flags.Sticky,
		&m.Meta.Flags.Open,
		&m.Meta.Flags.Deleted,
		&m.Meta.Flags.Moderated,
		&m.Meta.Flags.Visible,
		&m.PollOpen,
		&m.Multi,
		&m.CommentCount,
		&m.ViewCount,
	)
	if err == sql.ErrNoRows {
		return PollSummaryType{}, http.StatusNotFound,
			fmt.Errorf("Resource with ID %d not found", id)

	} else if err != nil {
		return PollSummaryType{}, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}

	if m.VotingEndsNullable.Valid {
		m.VotingEnds = m.VotingEndsNullable.Time.Format(time.RFC3339Nano)
	}
	m.Meta.Links =
		[]h.LinkType{
			h.GetLink("self", "", h.ItemTypePoll, m.ID),
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

// GetPolls returns a collection of polls
func GetPolls(
	siteID int64,
	profileID int64,
	limit int64,
	offset int64,
) (
	[]PollSummaryType,
	int64,
	int64,
	int,
	error,
) {

	// Retrieve resources
	db, err := h.GetConnection()
	if err != nil {
		return []PollSummaryType{}, 0, 0, http.StatusInternalServerError, err
	}

	rows, err := db.Query(`
SELECT COUNT(*) OVER() AS total
      ,p.poll_id
  FROM polls p
      ,microcosms m
 WHERE p.microcosm_id = m.microcosm_id
   AND m.site_id = $1
   AND m.is_deleted IS NOT TRUE
   AND m.is_moderated IS NOT TRUE
   AND p.is_deleted IS NOT TRUE
   AND p.is_moderated IS NOT TRUE
 ORDER BY p.created ASC
 LIMIT $2
OFFSET $3`,
		siteID,
		limit,
		offset,
	)
	if err != nil {
		return []PollSummaryType{}, 0, 0, http.StatusInternalServerError,
			fmt.Errorf("Database query failed: %v", err.Error())
	}
	defer rows.Close()

	var ems []PollSummaryType

	var total int64
	for rows.Next() {
		var id int64
		err = rows.Scan(
			&total,
			&id,
		)
		if err != nil {
			return []PollSummaryType{}, 0, 0, http.StatusInternalServerError,
				fmt.Errorf("Row parsing error: %v", err.Error())
		}

		m, status, err := GetPollSummary(siteID, id, profileID)
		if err != nil {
			return []PollSummaryType{}, 0, 0, status, err
		}

		ems = append(ems, m)
	}
	err = rows.Err()
	if err != nil {
		return []PollSummaryType{}, 0, 0, http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows: %v", err.Error())
	}
	rows.Close()

	pages := h.GetPageCount(total, limit)
	maxOffset := h.GetMaxOffset(total, limit)

	if offset > maxOffset {
		return []PollSummaryType{}, 0, 0, http.StatusBadRequest,
			fmt.Errorf("not enough records, "+
				"offset (%d) would return an empty page.", offset)
	}

	return ems, total, pages, http.StatusOK, nil
}

package models

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/lib/pq"

	merrors "github.com/microcosm-cc/microcosm/errors"
	h "github.com/microcosm-cc/microcosm/helpers"
)

// 2 queries are run, so a 5 second timeout would mean a worst case of 10
// seconds and so on
const searchTimeout = 15 * time.Second

func searchFullText(
	siteID int64,
	searchURL url.URL,
	profileID int64,
	m SearchResults,
) (
	SearchResults,
	int,
	error,
) {

	limit, offset, status, err := h.GetLimitAndOffset(searchURL.Query())
	if err != nil {
		glog.Errorf("h.GetLimitAndOffset(searchURL.Query()) %+v", err)
		return m, status, err
	}

	start := time.Now()

	// Search options
	var joinEvents bool

	orderBy := `rank DESC
         ,f.last_modified DESC`

	switch m.Query.Sort {
	case "date":
		orderBy = `f.last_modified DESC`
	case "oldest":
		joinEvents = true
		orderBy = `e."when" ASC`
	case "newest":
		joinEvents = true
		orderBy = `e."when" DESC`
	}

	var filterFollowingJoin string
	if m.Query.Following {
		filterFollowingJoin = `
                  JOIN watchers w ON w.item_type_id = f.item_type_id
                                 AND w.item_id = f.item_id
                                 AND w.profile_id = $2`
	}

	fullTextScope := `document`
	var filterTitle string
	if m.Query.InTitle {
		fullTextScope = `title`
		filterTitle = `
              AND f.item_type_id <> 4`
	}

	var filterItemTypes string
	var filterItems string
	var includeComments bool
	if !m.Query.InTitle {
		includeComments = true
	}

	if len(m.Query.ItemTypeIDs) > 0 {
		var itemTypeInList []string
		var itemTypeSansCommentsInList []string

		// Take care of the item types
		for _, v := range m.Query.ItemTypeIDs {
			switch v {
			case h.ItemTypes[h.ItemTypeComment]:
				includeComments = true
				itemTypeInList = append(itemTypeInList, strconv.FormatInt(v, 10))
			default:
				itemTypeInList = append(itemTypeInList, strconv.FormatInt(v, 10))
				itemTypeSansCommentsInList = append(itemTypeSansCommentsInList, strconv.FormatInt(v, 10))
			}
		}

		if len(m.Query.ItemIDs) == 0 {
			if len(m.Query.ItemTypeIDs) == 1 {
				filterItemTypes = fmt.Sprintf(`
              AND f.item_type_id = %d`,
					m.Query.ItemTypeIDs[0],
				)
			} else {
				if includeComments {
					filterItemTypes = `
              AND (   (f.item_type_id IN (` + strings.Join(itemTypeSansCommentsInList, `,`) + `))
                   OR (f.item_type_id = 4 AND f.parent_item_type_id IN (` + strings.Join(itemTypeSansCommentsInList, `,`) + `))
                 )`
				} else {
					filterItemTypes = `
              AND f.item_type_id IN (` + strings.Join(itemTypeInList, `,`) + `)`
				}
			}
		}

		// Take care of the item ids, which are only valid when we have item
		// types
		if len(m.Query.ItemIDs) > 0 {
			var itemIdsInList []string
			for _, v := range m.Query.ItemIDs {
				itemIdsInList = append(itemIdsInList, strconv.FormatInt(v, 10))
			}

			if len(m.Query.ItemIDs) == 1 {
				if includeComments {
					filterItems = fmt.Sprintf(`
              AND (   (si.item_type_id IN (`+strings.Join(itemTypeSansCommentsInList, `,`)+`) AND si.item_id = %d)
                   OR (si.item_type_id = 4 AND si.parent_item_id = %d AND si.parent_item_type_id IN (`+strings.Join(itemTypeSansCommentsInList, `,`)+`))
                  )`,
						m.Query.ItemIDs[0],
						m.Query.ItemIDs[0],
					)
				} else {
					filterItems = fmt.Sprintf(`
              AND si.item_id = %d`,
						m.Query.ItemIDs[0],
					)
				}
			} else {
				if includeComments {
					filterItems = `
              AND (   (si.item_type_id IN (` + strings.Join(itemTypeSansCommentsInList, `,`) + `) AND si.item_id IN (` + strings.Join(itemIdsInList, `,`) + `))
                   OR (si.item_type_id = 4 AND si.parent_item_type_id IN (` + strings.Join(itemTypeSansCommentsInList, `,`) + `) AND si.parent_item_id IN (` + strings.Join(itemIdsInList, `,`) + `))
                  )`
				} else {
					filterItems = `
              AND si.item_type_id IN (` + strings.Join(itemTypeInList, `,`) + `)
              AND si.item_id IN (` + strings.Join(itemIdsInList, `,`) + `)`
				}
			}
		}
	}

	// Note: hashtags being inserted into the query this way may appear
	// initially to be a vector for a SQL injection attack. However the
	// source of these hashtags is a regexp in hashtags.go which only
	// matches contiguous alphanum strings and does not permit spaces,
	// quotes, semicolons or any other escapable sequence that can be
	// utilised to create an attack.
	var filterHashTag string
	for _, hashtag := range m.Query.Hashtags {
		filterHashTag += `
              AND si.` + fullTextScope + `_text ~* '\W` + hashtag + `\W'`
	}

	var filterProfileID string
	if m.Query.ProfileID > 0 {
		filterProfileID = fmt.Sprintf(`
              AND si.profile_id = %d`, m.Query.ProfileID)
	}

	var (
		filterMicrocosmIDs    string
		filterHuddlesJoin     string
		filterThingsInHuddles string
	)
	if len(m.Query.MicrocosmIDs) > 0 {
		if len(m.Query.MicrocosmIDs) == 1 {
			filterMicrocosmIDs = fmt.Sprintf(`
   AND f.microcosm_id = %d`, m.Query.MicrocosmIDs[0])
		} else {
			var inList = ``

			for i, v := range m.Query.MicrocosmIDs {
				inList += strconv.FormatInt(v, 10)
				if i < len(m.Query.MicrocosmIDs)-1 {
					inList += `,`
				}
			}
			filterMicrocosmIDs = `
   AND f.microcosm_id IN (` + inList + `)`
		}
	} else {
		filterHuddlesJoin = `
             LEFT JOIN huddle_profiles h ON (f.parent_item_type_id = 5 OR f.item_type_id = 5)
                                        AND h.huddle_id = COALESCE(f.parent_item_id, f.item_id)
                                        AND h.profile_id = $2`
		filterThingsInHuddles = `
                   OR -- Things in huddles
                      COALESCE(f.parent_item_id, f.item_id) = h.huddle_id`
	}

	var filterModified string
	if !m.Query.SinceTime.IsZero() || !m.Query.UntilTime.IsZero() {

		if m.Query.UntilTime.IsZero() {
			filterModified = fmt.Sprintf(`
   AND f.last_modified > to_timestamp(%d)`,
				m.Query.SinceTime.Unix(),
			)

		} else if m.Query.SinceTime.IsZero() {
			filterModified = fmt.Sprintf(`
   AND f.last_modified < to_timestamp(%d)`,
				m.Query.UntilTime.Unix(),
			)
		} else {
			filterModified = fmt.Sprintf(`
   AND f.last_modified BETWEEN to_timestamp(%d) AND to_timestamp(%d)`,
				m.Query.SinceTime.Unix(),
				m.Query.UntilTime.Unix(),
			)
		}
	}

	var (
		filterEventsJoin  string
		filterEventsWhere string
	)
	if !m.Query.EventAfterTime.IsZero() || !m.Query.EventBeforeTime.IsZero() {
		joinEvents = true

		if m.Query.EventBeforeTime.IsZero() {
			filterModified = fmt.Sprintf(`
   AND e."when"::TIMESTAMP AT TIME ZONE e.tz_name > to_timestamp(%d)`,
				m.Query.EventAfterTime.Unix(),
			)

		} else if m.Query.EventAfterTime.IsZero() {
			filterModified = fmt.Sprintf(`
   AND e."when"::TIMESTAMP AT TIME ZONE e.tz_name < to_timestamp(%d)`,
				m.Query.EventBeforeTime.Unix(),
			)
		} else {
			filterModified = fmt.Sprintf(`
   AND e."when"::TIMESTAMP AT TIME ZONE e.tz_name BETWEEN to_timestamp(%d) AND to_timestamp(%d)`,
				m.Query.EventAfterTime.Unix(),
				m.Query.EventBeforeTime.Unix(),
			)
		}
	}

	if joinEvents || m.Query.Attendee {
		filterEventsJoin = `
       JOIN events e ON e.event_id = f.item_id`

		if m.Query.Attendee {
			filterEventsJoin += `
       JOIN attendees a ON a.event_id = e.event_id
                       AND a.profile_id = ` + strconv.FormatInt(profileID, 10) + `
                       AND a.state_id = 1`
		}
	}

	var filterHasAttachmentsJoin string
	if len(m.Query.Has) > 0 {
		for _, val := range m.Query.Has {
			switch val {
			case "attachment":
				filterHasAttachmentsJoin = `
  JOIN (SELECT DISTINCT item_type_id, item_id FROM attachments) AS a
           ON a.item_type_id = f.item_type_id
          AND a.item_id = f.item_id`
			default:
			}
		}
	}

	var filterSRToMicrocosms string
	if filterMicrocosmIDs != "" {
		filterSRToMicrocosms = `
       AND si.microcosm_id IN (SELECT microcosm_id FROM m)`
	}

	var filterSRToItemTypes string
	if filterItemTypes != "" {
		filterSRToItemTypes = strings.Replace(
			strings.Replace(
				filterItemTypes,
				`f.item_type_id`,
				`si.item_type_id`,
				-1,
			),
			`f.parent_item_type_id`,
			`si.parent_item_type_id`,
			-1,
		)
	}

	var filterSRHasAttachmentsJoin string
	if filterHasAttachmentsJoin != "" {
		filterSRHasAttachmentsJoin = strings.Replace(filterHasAttachmentsJoin, `f.item`, `si.item`, -1)
	}

	// Now we define our SQL
	sqlQuery := `
WITH m AS (
    SELECT m.microcosm_id
      FROM microcosms m
      LEFT JOIN permissions_cache p ON p.site_id = m.site_id
                                   AND p.item_type_id = 2
                                   AND p.item_id = m.microcosm_id
                                   AND p.profile_id = $2
      LEFT JOIN ignores_expanded i ON i.profile_id = $2
                                  AND i.item_type_id = 2
                                  AND i.item_id = m.microcosm_id
     WHERE m.site_id = $1
       AND m.is_deleted IS NOT TRUE
       AND m.is_moderated IS NOT TRUE
       AND i.profile_id IS NULL` + strings.Replace(filterMicrocosmIDs, `f.microcosm_id`, `m.microcosm_id`, -1) + `
       AND (
               (p.can_read IS NOT NULL AND p.can_read IS TRUE)
            OR (get_effective_permissions($1,m.microcosm_id,2,m.microcosm_id,$2)).can_read IS TRUE
           )
), sr AS (
    SELECT si.item_type_id
          ,si.item_id
      FROM search_index si` + filterSRHasAttachmentsJoin + `
          ,plainto_tsquery($3) AS query
     WHERE si.site_id = $1
       AND si.` + fullTextScope + `_vector @@ query` +
		filterSRToMicrocosms +
		filterSRToItemTypes + `
)
SELECT total
      ,item_type_id
      ,item_id
      ,parent_item_type_id
      ,parent_item_id
      ,last_modified
      ,rank
      ,ts_headline(` + fullTextScope + `_text, query) AS highlight
      ,has_unread(item_type_id, item_id, $2)
  FROM (
           SELECT COUNT(*) OVER() AS total
                 ,f.item_type_id
                 ,f.item_id
                 ,f.parent_item_type_id
                 ,f.parent_item_id
                 ,f.last_modified
                 ,ts_rank_cd(si.` + fullTextScope + `_vector, query, 8) AS rank
                 ,si.` + fullTextScope + `_text
                 ,query.query
             FROM sr
                  JOIN search_index si ON sr.item_type_id = si.item_type_id
                                      AND sr.item_id = si.item_id
                  JOIN flags f ON f.item_type_id = si.item_type_id
                              AND f.item_id = si.item_id
             LEFT JOIN ignores i ON i.profile_id = $2
                                AND i.item_type_id = f.item_type_id
                                AND i.item_id = f.item_id` +
		filterHasAttachmentsJoin +
		filterEventsJoin +
		filterFollowingJoin +
		filterHuddlesJoin + `
                 ,plainto_tsquery($3) AS query
            WHERE f.site_id = $1
              AND i.profile_id IS NULL` +
		filterModified +
		filterMicrocosmIDs +
		filterTitle +
		filterItemTypes +
		filterItems +
		filterHashTag +
		filterEventsWhere +
		filterProfileID + `
              AND f.microcosm_is_deleted IS NOT TRUE
              AND f.microcosm_is_moderated IS NOT TRUE
              AND f.parent_is_deleted IS NOT TRUE
              AND f.parent_is_moderated IS NOT TRUE
              AND f.item_is_deleted IS NOT TRUE
              AND f.item_is_moderated IS NOT TRUE
              AND (
                      -- Things that are public by default
                      COALESCE(f.parent_item_type_id, f.item_type_id) = 3
                   OR -- Things in microcosms
                      COALESCE(f.microcosm_id, f.item_id) IN (SELECT microcosm_id FROM m)` + filterThingsInHuddles + `
                  )
            ORDER BY ` + orderBy + `
            LIMIT $4
           OFFSET $5
       ) r
`

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return m, http.StatusInternalServerError, err
	}

	// The main query
	queryID := `Search` + randomString()
	queryTimer := time.NewTimer(searchTimeout)
	go func() {
		<-queryTimer.C
		db.Exec(`SELECT pg_cancel_backend(pid)
  FROM pg_stat_activity
 WHERE state = 'active'
   AND query LIKE '--` + queryID + `%'`)
	}()
	// This nested query is used to run the `has_unread` query on only the rows
	// that are returned, rather than on all rows in the underlying query before
	// limit has been applied.
	rows, err := db.Query(
		`--`+queryID+
			sqlQuery,
		siteID,
		profileID,
		m.Query.Query,
		limit,
		offset,
	)
	queryTimer.Stop()
	if err != nil {
		e, ok := err.(*pq.Error)

		if !ok {
			glog.Errorf(
				"stmt.Query(%d, %s, %d, %d, %d) %+v",
				siteID,
				m.Query.Query,
				profileID,
				limit,
				offset,
				err,
			)
			return m, http.StatusInternalServerError,
				fmt.Errorf("Database query failed")
		}

		switch e.Code.Name() {
		case "query_canceled":
			glog.Errorf(
				"Query for '%s' took too long",
				m.Query.Query,
			)
			return m, http.StatusInternalServerError,
				merrors.MicrocosmError{
					ErrorCode:    24,
					ErrorMessage: "Too many results to process, please make your search more specific.",
				}
		default:
			glog.Errorf(
				"stmt.Query(%d, %s, %d, %d, %d) %+v",
				siteID,
				m.Query.Query,
				profileID,
				limit,
				offset,
				err,
			)
			return m, http.StatusInternalServerError,
				fmt.Errorf("Database query failed")
		}
	}
	defer rows.Close()

	var total int64
	rs := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		err = rows.Scan(
			&total,
			&r.ItemTypeID,
			&r.ItemID,
			&r.ParentItemTypeID,
			&r.ParentItemID,
			&r.LastModified,
			&r.Rank,
			&r.Highlight,
			&r.Unread,
		)
		if err != nil {
			glog.Errorf("rows.Scan() %+v", err)
			return m, http.StatusInternalServerError,
				fmt.Errorf("Row parsing error")
		}

		itemType, err := h.GetMapStringFromInt(h.ItemTypes, r.ItemTypeID)
		if err != nil {
			glog.Errorf(
				"h.GetMapStringFromInt(h.ItemTypes, %d) %+v",
				r.ItemTypeID,
				err,
			)
			return m, http.StatusInternalServerError, err
		}
		r.ItemType = itemType

		if r.ParentItemTypeID.Valid {
			parentItemType, err :=
				h.GetMapStringFromInt(h.ItemTypes, r.ParentItemTypeID.Int64)
			if err != nil {
				glog.Errorf(
					"h.GetMapStringFromInt(h.ItemTypes, %d) %+v",
					r.ParentItemTypeID.Int64,
					err,
				)
				return m, http.StatusInternalServerError, err
			}
			r.ParentItemType = parentItemType
		}

		rs = append(rs, r)
	}
	err = rows.Err()
	if err != nil {
		glog.Errorf("rows.Err() %+v", err)
		return m, http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows")
	}
	rows.Close()

	pages := h.GetPageCount(total, limit)
	maxOffset := h.GetMaxOffset(total, limit)

	if offset > maxOffset {
		glog.Infoln("offset > maxOffset")
		return m, http.StatusBadRequest,
			fmt.Errorf("not enough records, "+
				"offset (%d) would return an empty page.", offset)
	}

	// Extract the summaries
	var wg1 sync.WaitGroup
	req := make(chan SummaryContainerRequest)
	defer close(req)

	seq := 0
	for i := 0; i < len(rs); i++ {
		go HandleSummaryContainerRequest(
			siteID,
			rs[i].ItemTypeID,
			rs[i].ItemID,
			profileID,
			seq,
			req,
		)
		seq++
		wg1.Add(1)

		if rs[i].ParentItemID.Valid && rs[i].ParentItemID.Int64 > 0 {
			go HandleSummaryContainerRequest(
				siteID,
				rs[i].ParentItemTypeID.Int64,
				rs[i].ParentItemID.Int64,
				profileID,
				seq,
				req,
			)
			seq++
			wg1.Add(1)
		}
	}

	resps := []SummaryContainerRequest{}
	for i := 0; i < seq; i++ {
		resp := <-req
		wg1.Done()
		resps = append(resps, resp)
	}
	wg1.Wait()

	for _, resp := range resps {
		if resp.Err != nil {
			return m, resp.Status, resp.Err
		}
	}

	sort.Sort(SummaryContainerRequestsBySeq(resps))

	seq = 0
	for i := 0; i < len(rs); i++ {

		rs[i].Item = resps[seq].Item.Summary
		seq++

		if rs[i].ParentItemID.Valid && rs[i].ParentItemID.Int64 > 0 {
			rs[i].ParentItem = resps[seq].Item.Summary
			seq++
		}
	}

	m.Results = h.ConstructArray(
		rs,
		"result",
		total,
		limit,
		offset,
		pages,
		&searchURL,
	)

	// return milliseconds
	m.TimeTaken = time.Now().Sub(start).Nanoseconds() / 1000000

	return m, http.StatusOK, nil

}

// Copyright (c) 2011 Dmitry Chestnykh
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.
//
// https://github.com/dchest/uniuri

func randomString() string {

	const (
		// Standard length of uniuri string to achive ~95 bits of entropy.
		length = 16
		// Length of uniurl string to achive ~119 bits of entropy, closest
		// to what can be losslessly converted to UUIDv4 (122 bits).
		UUIDLen = 20
	)

	// Standard characters allowed in uniuri string.
	var chars = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

	// NewLenChars returns a new random string of the provided length, consisting
	// of the provided byte slice of allowed characters (maximum 256).

	b := make([]byte, length)
	r := make([]byte, length+(length/4)) // storage for random bytes.
	clen := byte(len(chars))
	maxrb := byte(256 - (256 % len(chars)))
	i := 0
	for {
		if _, err := io.ReadFull(rand.Reader, r); err != nil {
			panic("error reading from random source: " + err.Error())
		}
		for _, c := range r {
			if c >= maxrb {
				// Skip this number to avoid modulo bias.
				continue
			}
			b[i] = chars[c%clen]
			i++
			if i == length {
				return string(b)
			}
		}
	}
}

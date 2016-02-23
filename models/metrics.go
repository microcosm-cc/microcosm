package models

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"

	conf "github.com/microcosm-cc/microcosm/config"
	h "github.com/microcosm-cc/microcosm/helpers"
)

var defaultLogoURL = "https://meta.microco.sm/static/themes/1/logo.png"

// MetricType describes the attributes of all sites on Microcosm
type MetricType struct {
	Timestamp      time.Time
	Pageviews      int32
	Visits         int32
	Uniques        int32
	NewProfiles    int32
	EditedProfiles int32
	TotalProfiles  int32
	Signins        int32
	Comments       int32
	Conversations  int32
	EngagedForums  int32
	TotalForums    int32
}

// GetMetrics fetchs the latest metrics
func GetMetrics() ([]MetricType, int, error) {
	db, err := h.GetConnection()
	if err != nil {
		return []MetricType{}, http.StatusInternalServerError, err
	}

	rows, err := db.Query(`
SELECT job_timestamp
      ,pageviews
      ,visits
      ,uniques
      ,new_profiles
      ,edited_profiles
      ,total_profiles
      ,signins
      ,comments
      ,conversations
      ,engaged_forums
      ,total_forums
  FROM metrics
 WHERE job_timestamp IN (
           SELECT MAX(job_timestamp)
             FROM metrics
            GROUP BY job_timestamp::date
       )
 ORDER BY job_timestamp ASC`)
	if err != nil {
		return []MetricType{}, http.StatusInternalServerError, err
	}
	defer rows.Close()

	var ems []MetricType

	for rows.Next() {
		var metric MetricType
		err = rows.Scan(
			&metric.Timestamp,
			&metric.Pageviews,
			&metric.Visits,
			&metric.Uniques,
			&metric.NewProfiles,
			&metric.EditedProfiles,
			&metric.TotalProfiles,
			&metric.Signins,
			&metric.Comments,
			&metric.Conversations,
			&metric.EngagedForums,
			&metric.TotalForums,
		)
		if err != nil {
			return []MetricType{}, http.StatusInternalServerError,
				fmt.Errorf("Row parsing error: %v", err.Error())
		}
		ems = append(ems, metric)
	}
	err = rows.Err()
	if err != nil {
		return []MetricType{}, http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows: %v", err.Error())
	}
	rows.Close()

	return ems, http.StatusOK, nil
}

// UpdateMetrics builds a new set of metrics
func UpdateMetrics() error {
	if conf.ConfigStrings[conf.Environment] != `prod` {
		glog.Info("dev environment, skipping creation of metrics")
		return nil
	}

	// Subtract 24 hours from now (there is no Sub function that returns a time
	// value).
	end := time.Now()
	start := end.Add(-time.Hour * 24) // Will overlap/underlap on DST boundaries

	// Invoke metrics functions, store the results
	pCreated, pEdited, pTotal, err := ProfileMetrics(start)
	if err != nil {
		glog.Errorf("UpdateMetrics: %v", err.Error())
		return err
	}

	fTotal, fEngaged, err := ForumMetrics()
	if err != nil {
		glog.Errorf("UpdateMetrics: %v", err.Error())
		return err
	}

	signins, comments, convs, err := UserGenMetrics(start)
	if err != nil {
		glog.Errorf("UpdateMetrics: %v", err.Error())
		return err
	}

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("UpdateMetrics: %v", err.Error())
		return err
	}

	_, err = db.Exec(`
INSERT INTO metrics(
	job_timestamp,
	new_profiles,
	edited_profiles,
	total_profiles,
	signins,

	comments,
	conversations,
	engaged_forums,
	total_forums
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,

	$6,
	$7,
	$8,
	$9
)`,
		end,
		pCreated,
		pEdited,
		pTotal,
		signins,

		comments,
		convs,
		fEngaged,
		fTotal,
	)
	if err != nil {
		glog.Errorf("UpdateMetrics: %+v", err)
		return err
	}

	// 	// This is a separate update for 2 reasons:
	// 	// 1) We are updating *all* rows from yesterday
	// 	// 2) If this throws an error, then at least we've already updated today's
	// 	//    stats
	// 	pageviews, visits, uniques, err := GoogleAnalytics(start, end)
	// 	if err != nil {
	// 		glog.Errorf("UpdateMetrics: %v", err.Error())
	// 		return err
	// 	}

	// 	_, err = db.Exec(`
	// UPDATE metrics
	//    SET pageviews = $1
	//       ,visits = $2
	//       ,uniques = $3
	//  WHERE job_timestamp::date = (NOW() - interval '1 day')::DATE;`,
	// 		pageviews,
	// 		visits,
	// 		uniques,
	// 	)
	// 	if err != nil {
	// 		glog.Errorf("UpdateMetrics: %+v", err)
	// 		return err
	// 	}

	return nil
}

// ProfileMetrics builds metrics about profiles
func ProfileMetrics(start time.Time) (created int, edited int, total int, err error) {

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("ProfileMetrics: %s", err.Error())
		return
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&total)
	if err != nil {
		return
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE created >= $1`,
		start,
	).Scan(
		&created,
	)
	if err != nil {
		return
	}

	err = db.QueryRow(`
SELECT COUNT(*)
  FROM profiles
 WHERE profile_name !~ 'user*'
   AND avatar_id IS NOT NULL`,
	).Scan(
		&edited,
	)

	return
}

// GoogleAnalytics fetches metrics from Google
func GoogleAnalytics(
	start time.Time,
	end time.Time,
) (
	int,
	int,
	int,
	error,
) {
	// // Dev env does not register on Google Analytics
	// if conf.ConfigStrings[conf.Environment] != `prod` {
	// 	glog.Info("dev environment, skipping creation of metrics")
	// 	return 0, 0, 0, nil
	// }

	// // This is where we put the auth code once we have it
	// code := "4/gMqQpQtF5u4h_-xeEH6v5iIDkNsG.4tytjKAb8TgSOl05ti8ZT3YD5W2xjQI"

	// config := &oauth2.Config{
	// 	ClientId:     "569955368064-507h6pojaqoick1pfh6of9j72fb8r9fv.apps.googleusercontent.com",
	// 	ClientSecret: "anB8qgvtEks4Pr2mc5Nfj4eL",
	// 	RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
	// 	Scope:        "https://www.googleapis.com/auth/analytics https://www.googleapis.com/auth/analytics.readonly",
	// 	AuthURL:      "https://accounts.google.com/o/oauth2/auth",
	// 	TokenURL:     "https://accounts.google.com/o/oauth2/token",
	// 	TokenCache:   oauth2.CacheFile("/etc/microcosm/ga-auth-cache.json"),
	// }

	// // Set up a Transport using the config.
	// transport := &oauth2.Transport{Config: config}

	// token, err := config.TokenCache.Token()
	// if err != nil {

	// 	if code == "" {
	// 		// Get an authorization code from the data provider.
	// 		// ("Please ask the user if I can access this resource.")
	// 		url := config.AuthCodeURL("")
	// 		glog.Error("GA metrics: Visit this URL to get a code, then run again with -code=YOUR_CODE\n" + url)
	// 		return 0, 0, 0, err
	// 	}

	// 	// Exchange the authorization code for an access token.
	// 	// ("Here's the code you gave the user, now give me a token!")
	// 	// If we already have a token in the cache but it has expired, this will
	// 	// refresh the token
	// 	token, err = transport.Exchange(code)
	// 	if err != nil {
	// 		glog.Errorf("GA metrics: Exchange: %+v", err)
	// 		return 0, 0, 0, err
	// 	}

	// 	// (The Exchange method will automatically cache the token.)
	// 	glog.Infof("GA metrics: Token is cached in %v\n", config.TokenCache)
	// }

	// // Make the actual request using the cached token to authenticate.
	// // ("Here's the token, let me in!")
	// transport.Token = token

	// // Make the request.
	// r, err := transport.Client().Get("https://www.googleapis.com/analytics/v3/data/ga?ids=ga%3A71862589&start-date=yesterday&end-date=yesterday&metrics=ga%3Apageviews%2Cga%3Asessions%2Cga%3Ausers&fields=query(end-date%2Cstart-date)%2CtotalsForAllResults")
	// if err != nil {
	// 	glog.Errorf("GA metrics: Get: %+v", err)
	// 	return 0, 0, 0, err
	// }
	// defer r.Body.Close()

	// type GA struct {
	// 	Query struct {
	// 		StartDate string `json:"start-date"`
	// 		EndDate   string `json:"end-date"`
	// 	} `json:"query"`
	// 	Totals struct {
	// 		Pageviews string `json:"ga:pageviews"`
	// 		Sessions  string `json:"ga:sessions"`
	// 		Users     string `json:"ga:users"`
	// 	} `json:"totalsForAllResults"`
	// }

	// ga := GA{}
	// err = json.NewDecoder(r.Body).Decode(&ga)
	// r.Body.Close()
	// if err != nil {
	// 	glog.Errorf("GA metrics: Decode: %+v", err)
	// 	return 0, 0, 0, err
	// }

	// pageviews, err := strconv.Atoi(ga.Totals.Pageviews)
	// if err != nil {
	// 	glog.Errorf("GA metrics: Atoi: %+v\n%+v", err, ga)
	// 	return 0, 0, 0, err
	// }

	// sessions, err := strconv.Atoi(ga.Totals.Sessions)
	// if err != nil {
	// 	glog.Errorf("GA metrics: Atoi: %+v", err)
	// 	return 0, 0, 0, err
	// }

	// users, err := strconv.Atoi(ga.Totals.Users)
	// if err != nil {
	// 	glog.Errorf("GA metrics: Atoi: %+v", err)
	// 	return 0, 0, 0, err
	// }

	var pageviews, sessions, users int
	return pageviews, sessions, users, nil
}

// ForumMetrics builds metrics about sites
func ForumMetrics() (total int, engaged int, err error) {
	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("ForumMetrics: %s", err.Error())
		return
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM sites`).Scan(&total)
	if err != nil {
		return
	}

	err = db.QueryRow(`
SELECT COUNT(*) FROM (
           SELECT s.site_id
             FROM sites s
                  JOIN microcosms m ON s.site_id = m.site_id
                  JOIN conversations c ON m.microcosm_id = c.microcosm_id
                  JOIN comments cm ON c.conversation_id = cm.item_id
            UNION
           SELECT s.site_id
             FROM sites s
                  JOIN microcosms m ON s.site_id = m.site_id
                  JOIN events e ON m.microcosm_id = e.microcosm_id
                  JOIN comments cm ON e.event_id = cm.item_id	
            WHERE logo_url !~ $1
              AND (
                      SELECT COUNT(*)
                        FROM profiles p
                       WHERE p.site_id = s.site_id
                  ) > 1
            GROUP BY s.site_id
       ) AS t`,
		defaultLogoURL,
	).Scan(
		&engaged,
	)

	return
}

// UserGenMetrics builds metrics about user activity
func UserGenMetrics(
	start time.Time,
) (
	signins int,
	comments int,
	convs int,
	err error,
) {

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("UserGenMetrics: %s", err.Error())
		return
	}

	err = db.QueryRow(
		`SELECT COUNT(*) FROM profiles where profiles.last_active > $1`,
		start,
	).Scan(
		&signins,
	)
	if err != nil {
		return
	}

	err = db.QueryRow(
		`SELECT COUNT(*) FROM comments WHERE created >= $1`,
		start,
	).Scan(
		&comments,
	)
	if err != nil {
		return
	}

	err = db.QueryRow(
		`SELECT COUNT(*) FROM conversations WHERE created >= $1`,
		start,
	).Scan(
		&convs,
	)
	return
}

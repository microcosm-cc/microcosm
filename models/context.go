package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/mux"

	"github.com/microcosm-cc/microcosm/cache"
	conf "github.com/microcosm-cc/microcosm/config"
	e "github.com/microcosm-cc/microcosm/errors"
	h "github.com/microcosm-cc/microcosm/helpers"
)

const rootSiteId int64 = 1

type Context struct {
	Request        *http.Request
	ResponseWriter http.ResponseWriter
	Site           SiteType
	Auth           AuthType
	RouteVars      map[string]string
	StartTime      time.Time
	IP             net.IP
}

type AuthType struct {
	UserId      int64
	ProfileId   int64
	IsSiteOwner bool
	IsBanned    bool
	Method      string
	AccessToken AccessTokenType
}

type StandardResponse struct {
	Context string      `json:"context"`
	Status  int         `json:"status"`
	Data    interface{} `json:"data"`
	Errors  []string    `json:"error"`
}

func (c *Context) GetItemTypeAndItemId() (string, int64, int64, int, error) {

	keys := []string{
		"comment_id",
		"conversation_id",
		"event_id",
		"huddle_id",
		"microcosm_id",
		"poll_id",
		"profile_id",
		"site_id",
		"update_id",
		"update_type_id",
		"user_id",
		"watcher_id",
	}

	var (
		itemType   string
		itemTypeId int64
		itemId     int64
		err        error
		exists     bool
	)

	for _, key := range keys {
		if id, exists := c.RouteVars[key]; exists {
			itemType = strings.Replace(key, "_id", "", -1)
			itemId, err = strconv.ParseInt(id, 10, 64)
			if err != nil {
				return itemType, itemTypeId, itemId, http.StatusBadRequest,
					errors.New(
						fmt.Sprintf(
							"The supplied %s ('%s') is not a number.",
							key,
							id,
						),
					)
			}

			break
		}
	}

	if itemId == 0 {
		return itemType, itemTypeId, itemId, http.StatusBadRequest,
			errors.New(
				fmt.Sprintf(
					"Item type not determinable from URL: %s",
					c.RouteVars,
				),
			)
	}

	if itemTypeId, exists = h.ItemTypes[itemType]; !exists {
		return itemType, itemTypeId, itemId, http.StatusBadRequest, errors.New(
			fmt.Sprintf("%s is not a valid item type", itemType),
		)
	}

	return itemType, itemTypeId, itemId, http.StatusOK, nil
}

func MakeContext(
	request *http.Request,
	responseWriter http.ResponseWriter,
) (
	*Context,
	int,
	error,
) {

	var c *Context = new(Context)
	c.Request = request
	c.ResponseWriter = responseWriter
	c.RouteVars = mux.Vars(request)
	c.StartTime = time.Now()
	c.IP = GetRequestIP(request)

	// Which site is this request for?
	err := c.getSiteContext()
	if err != nil {
		return c, http.StatusNotFound, e.New(
			0,
			0,
			"context.MakeContext",
			e.SiteNotFound,
			fmt.Sprintf("No site context loaded for host: %s", err.Error()),
		)
	}

	status, err := c.authenticate()
	if err != nil {
		c.Auth.UserId = -1
		return c, status, err
	}

	return c, http.StatusOK, nil
}

func GetRequestIP(request *http.Request) net.IP {
	host, _, _ := net.SplitHostPort(request.RemoteAddr)
	return net.ParseIP(host)
}

func (c *Context) authenticate() (int, error) {

	// Authorisation is accepted by query string or header
	atQuery := c.Request.URL.Query().Get("access_token")
	atHeader := c.Request.Header.Get("Authorization")
	var accessToken string

	// Expected header is: "Authorization: Bearer access_token"
	if atHeader != "" {
		authParts := strings.Split(strings.Trim(atHeader, " "), " ")

		if len(authParts) != 2 {
			// Should be two parts, return indicator for bad token
			glog.Warningf(`AccessToken must have two parts: %s`, atHeader)
			return http.StatusUnauthorized, errors.New("Invalid access token")
		}

		if authParts[0] != "Bearer" {
			// Should start with 'Bearer', return indicator for bad token
			glog.Warningf(`AccessToken must have Bearer header: %s`, atHeader)
			return http.StatusUnauthorized,
				errors.New("Authorization header must be " +
					"in the format 'Bearer access_token'")
		}

		accessToken = authParts[1]
		c.Auth.Method = "header"

	} else if atQuery != "" {
		accessToken = atQuery
		c.Auth.Method = "query"
	}

	// Since the request URL is reused, trim access_token if present
	query := c.Request.URL.Query()

	if query.Get("access_token") != "" {
		query.Del("access_token")
		c.Request.URL.RawQuery = query.Encode()
	}

	if accessToken != "" {
		// Verify access token by fetching it from storage
		storedToken, _, err := GetAccessToken(accessToken)
		if err != nil {
			c.Auth.UserId = -1
			glog.Warningf(`Invalid access token: %s  %+v`, accessToken, err)
			return http.StatusUnauthorized,
				errors.New("Invalid (bad or expired) access token")
		}

		c.Auth.AccessToken = storedToken
		c.Auth.UserId = c.Auth.AccessToken.UserId

		// Fetch user profile
		profile, status, err :=
			GetOrCreateProfile(c.Site, c.Auth.AccessToken.User)
		if err != nil {
			c.Auth.UserId = -1

			glog.Warningf(
				`GetOrCreateProfile: %+v  %+v`,
				c.Auth.AccessToken.User,
				err,
			)
			return status, errors.New(
				fmt.Sprintf(
					"%+v %+v %+v %s",
					c.Site,
					c.Auth.AccessToken.User,
					profile,
					err,
				),
			)
		}
		c.Auth.ProfileId = profile.ID

		// Check to see if banned before finishing the profile assignment.
		// A banned person can never sign in.
		// Note: We cannot block the URLs required to show an empty error page.
		if !(c.Request.URL.Path == `/api/v1/site` ||
			c.Request.URL.Path == `/api/v1/whoami` ||
			c.Request.URL.Path == fmt.Sprintf(`/api/v1/profiles/%d`, profile.ID)) &&
			IsBanned(c.Site.ID, storedToken.UserId) {

			c.Auth.IsBanned = true
			c.Auth.UserId = -1
			return http.StatusForbidden, fmt.Errorf("Banned")
		}

		// Update entry for user's last activity
		if c.Auth.ProfileId > 0 {
			lastActiveKey := fmt.Sprintf("la_%d", c.Auth.ProfileId)
			_, ok := cache.CacheGetInt64(lastActiveKey)
			if !ok {
				// Background as the first call to this is likely a whoami which
				// is a blocking call
				go UpdateLastActive(c.Auth.ProfileId, c.StartTime)

				// Only update every 60 seconds at most
				cache.CacheSetInt64(lastActiveKey, 1, 60)
			}
		}

		// Determine whether user is site owner
		if c.Site.ID > 0 && c.Site.OwnedByID == profile.ID {
			c.Auth.IsSiteOwner = true
		}
	}

	return http.StatusOK, nil
}

func MakeEmptyContext(
	request *http.Request,
	responseWriter http.ResponseWriter,
) (
	*Context,
	int,
	error,
) {

	var c *Context = new(Context)
	c.Request = request
	c.ResponseWriter = responseWriter
	c.RouteVars = mux.Vars(request)
	c.StartTime = time.Now()
	c.IP = GetRequestIP(request)

	return c, http.StatusOK, nil
}

func (c *Context) getSiteContext() error {

	// Ignore port
	host := strings.Split(c.Request.Host, ":")[0]
	hostParts := strings.Split(host, ".")
	mcDomain := conf.CONFIG_STRING[conf.KEY_MICROCOSM_DOMAIN]

	var err error
	if host == mcDomain {
		// Request is for the root site (http://microco.sm) which has ID 1
		c.Site, _, err = GetSite(rootSiteId)
		if err != nil {
			return err
		}

	} else if len(hostParts) == 3 &&
		strings.Join(hostParts[1:], ".") == mcDomain {
		// Request is for site.microco.sm, so fetch by subdomain key
		c.Site, _, err = GetSiteBySubdomain(hostParts[0])
		if err != nil {
			return err
		}

		// If this is the root site, then we shouldn't be accessed via
		// root.microco.sm and being accessed via microco.sm was already handled
		// above. We'll claim we don't exist.
		if c.Site.ID == rootSiteId {
			return errors.New("Unknown site requested")
		}

		// If the site has subsequently been deleted, we should pretend that we
		// know nothing about it.
		if c.Site.Meta.Flags.Deleted {
			return errors.New("Unknown site requested")
		}

	} else {
		return errors.New("Unknown site requested")
	}

	return nil
}

func (c *Context) GetHttpMethod() string {
	m := c.Request.Method

	if m == "POST" {
		if c.Request.Header.Get("X-HTTP-Method-Override") != "" {
			m = strings.ToUpper(c.Request.Header.Get("X-HTTP-Method-Override"))
		}
		if c.Request.URL.Query().Get("method") != "" {
			m = strings.ToUpper(c.Request.URL.Query().Get("method"))
		}

		switch m {
		case "CONNECT":
		case "DELETE":
		case "GET":
		case "HEAD":
		case "OPTIONS":
		case "PATCH":
		case "POST":
		case "PUT":
		case "TRACE":
		default:
			// If it wasn't one of the above then let's just use what we know
			// is safe
			return c.Request.Method
		}
	}

	return m
}

func (c *Context) IsRootSite() bool {
	if c.Site.SubdomainKey == "root" {
		return true
	} else {
		return false
	}
}

func (c *Context) Respond(
	data interface{},
	statusCode int,
	errors []string,
	context *Context,
) error {

	// make the standard response object
	obj := StandardResponse{
		Context: c.Request.URL.Query().Get("context"),
		Status:  statusCode,
		Data:    data,
		Errors:  errors,
	}

	// Prevent content type detection, a.k.a. sniffing
	c.ResponseWriter.Header().Set("Content-Type", "application/json")
	c.ResponseWriter.Header().Set("Access-Control-Allow-Origin", "*")

	// Cache headers
	if c.Auth.ProfileId == 0 &&
		statusCode == http.StatusOK &&
		c.GetHttpMethod() == "GET" {
		// Public, cache for a short while
		c.ResponseWriter.Header().Set(`Cache-Control`, `public, max-age=300`)
		c.ResponseWriter.Header().Set(`Vary`, `Authorization`)
	} else {
		// Potentially private, do not cache
		c.ResponseWriter.Header().Set(`Cache-Control`, `no-cache, max-age=0`)
		c.ResponseWriter.Header().Set(`Vary`, `Authorization`)
	}

	// format the output
	output, err := FormatAsJson(c, obj)
	if err != nil {
		http.Error(c.ResponseWriter, err.Error(), http.StatusInternalServerError)
		return err
	}

	// Prevent chunking
	contentLength := len(string(output))
	c.ResponseWriter.Header().Set("Content-Length", strconv.Itoa(contentLength))

	// Debugging info
	dur := time.Now().Sub(c.StartTime)
	go SendUsage(c, statusCode, contentLength, dur, errors)

	return c.WriteResponse(output, statusCode)
}

// This ultimately does the job of writing the response
func (c *Context) WriteResponse(output []byte, statusCode int) error {

	// Set status and write (finalise) all headers
	if strings.Index(c.Request.URL.String(), "always200") > -1 ||
		c.Request.Header.Get("X-Always-200") != "" {

		c.ResponseWriter.WriteHeader(http.StatusOK)
	} else {
		c.ResponseWriter.WriteHeader(statusCode)
	}

	// HEAD requests return no body and are used to check headers for cache
	// invalidation functions
	if c.GetHttpMethod() == "HEAD" {
		return nil
	}

	_, err := c.ResponseWriter.Write(output)

	// We only log at error severity when an error is not the result of the
	// client disconnecting. "broken pipe" is a syscall.EPIPE error that
	// indicates client disconnection.
	if err != nil {
		opErr, ok := err.(*net.OpError)
		if !ok || opErr.Err != syscall.EPIPE {

			// Totally unexpected, definitely error
			glog.Errorf(
				"Error writing %s response to %s : %+v\n",
				c.GetHttpMethod(),
				c.Request.URL.String(),
				err,
			)
			return err

		} else {

			// Broken pipe, which is expected, but we log as warning in case
			// multiple clients do this at once and it hints at network issues
			glog.Warningf(
				"Error writing %s response to %s : %+v\n",
				c.GetHttpMethod(),
				c.Request.URL.String(),
				err,
			)
			return err
		}
	}

	return nil
}

func (c *Context) RespondWithOptions(options []string) error {
	c.ResponseWriter.Header().Set("Allow", strings.Join(options, ","))
	c.ResponseWriter.Header().Set("Content-Length", "0")
	c.ResponseWriter.WriteHeader(http.StatusOK)
	return nil
}

// Responds with custom status code and an empty StandardResponse struct
func (c *Context) RespondWithStatus(statusCode int) error {
	return c.Respond(nil, statusCode, nil, c)
}

// Responds with the specified HTTP status code defined in RFC 2616
// and adds the status description to the errors list
// see http://golang.org/src/pkg/http/status.go for options
func (c *Context) RespondWithError(statusCode int) error {
	return c.RespondWithErrorMessage(http.StatusText(statusCode), statusCode)
}

// Responds with custom code and an error message
func (c *Context) RespondWithErrorMessage(
	message string,
	statusCode int,
) error {

	return c.Respond(nil, statusCode, []string{message}, c)
}

// RespondWithErrorDetail responds with detailed error code and message in the
// "data" object.
func (c *Context) RespondWithErrorDetail(err error, statusCode int) error {
	return c.Respond(err, statusCode, []string{err.Error()}, c)
}

// Responds with the specified data
func (c *Context) RespondWithData(data interface{}) error {
	return c.Respond(data, http.StatusOK, nil, c)
}

// Responds with OK status (200) and no data
func (c *Context) RespondWithOK() error {
	return c.RespondWithData(nil)
}

// Responds with 301 Permanently Moved (perm redirect)
func (c *Context) RespondWithMoved(location string) error {
	c.ResponseWriter.Header().Set("Location", location)
	return c.RespondWithStatus(http.StatusMovedPermanently)
}

// Responds with 303 See Other (created redirect)
func (c *Context) RespondWithSeeOther(location string) error {
	c.ResponseWriter.Header().Set("Location", location)

	return c.RespondWithStatus(http.StatusFound)
}

// Responds with 307 Temporarily Moved (temp redirect)
func (c *Context) RespondWithLocation(location string) error {
	c.ResponseWriter.Header().Set("Location", location)
	return c.RespondWithStatus(http.StatusTemporaryRedirect)
}

// Responds with 404 Not Found
func (c *Context) RespondWithNotFound() error {
	return c.RespondWithError(http.StatusNotFound)
}

// Responds with 501 Not Implemented
func (c *Context) RespondWithNotImplemented() error {
	return c.RespondWithError(http.StatusNotImplemented)
}

func FormatAsJson(c *Context, input interface{}) ([]byte, error) {
	// marshal json
	var output []byte
	var err error

	if strings.Index(c.Request.URL.String(), "disableBoiler") > -1 ||
		c.Request.Header.Get("X-Disable-Boiler") != "" {
		// If disableBoiler is set, then just render the value of the
		// data field
		respObj := reflect.Indirect(reflect.ValueOf(input))
		data := respObj.FieldByName("Data").Interface()

		output, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	} else {
		output, err = json.Marshal(input)
		if err != nil {
			return nil, err
		}
	}

	// JSONP
	if callback := c.Request.URL.Query().Get("callback"); callback != "" {
		requestContext := c.Request.URL.Query().Get("context")

		// wrap in js function
		outputString := callback + "(" + string(output)

		// pass the request context as the second param
		if requestContext != "" {
			outputString = outputString + ", \"" + requestContext + "\")"
		} else {
			outputString = outputString + ")"
		}

		// convert back
		output = []byte(outputString)

	}

	// This line puts a newline char at the end of the output, thus making
	// cURL requests nicer on the command line.
	// output = append(output, []byte("\n")...)

	c.ResponseWriter.Header().Set("Content-Type", "application/json")
	c.ResponseWriter.Header().Set("Content-Length", strconv.Itoa(len(output)))

	return output, nil
}

// types that impliment RequestDecoder can unmarshal
// the request body into an apropriate type/struct
type RequestDecoder interface {
	Unmarshal(cx *Context, v interface{}) error
}

// a JSON decoder for request body (just a wrapper to json.Unmarshal)
type JsonRequestDecoder struct{}

func (d *JsonRequestDecoder) Unmarshal(cx *Context, v interface{}) error {
	// read body
	err := json.NewDecoder(cx.Request.Body).Decode(&v)
	cx.Request.Body.Close()
	return err
}

// a form-enc decoder for request body
type FormRequestDecoder struct{}

func (d *FormRequestDecoder) Unmarshal(cx *Context, v interface{}) error {
	if cx.Request.Form == nil {
		cx.Request.ParseForm()
	}
	return UnmarshalForm(cx.Request.Form, v)
}

// map of Content-Type -> RequestDecoders
var decoders map[string]RequestDecoder = map[string]RequestDecoder{
	"application/json":                  new(JsonRequestDecoder),
	"application/x-www-form-urlencoded": new(FormRequestDecoder),
}

// context.Context Helper function to fill a variable with the contents
// of the request body. The body will be decoded based on the
// content-type and an apropriate RequestDecoder automatically selected
func (cx *Context) Fill(v interface{}) error {
	// get content type
	ct := cx.Request.Header.Get("Content-Type")
	// default to urlencoded
	if strings.Trim(ct, " ") == "" {
		ct = "application/x-www-form-urlencoded"
	}

	// ignore charset (after ';')
	ct = strings.Split(ct, ";")[0]
	// get request decoder
	decoder, ok := decoders[ct]
	if ok != true {
		return fmt.Errorf("Cannot decode request for %s data", ct)
	}
	// decode
	err := decoder.Unmarshal(cx, v)
	if err != nil {
		return err
	}
	// all clear
	return nil
}

// Fill a struct `v` from the values in `form`
func UnmarshalForm(form url.Values, v interface{}) error {

	// TODO(buro9) 2014-02-13: This currently uses the internal Go struct field
	// names and therefore is liable to break in the future.
	// We should read the existing json tags on the struct fields, and if they
	// match the passed-in value case-insensitively, that should be the only
	// way to populate the struct.

	// check v is valid
	rv := reflect.ValueOf(v).Elem()
	// dereference pointer
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	// get type
	rt := rv.Type()

	if rv.Kind() == reflect.Struct {
		// for each struct field on v
		for i := 0; i < rt.NumField(); i++ {
			err := unmarshalField(form, rt.Field(i), rv.Field(i))
			if err != nil {
				return err
			}
		}
	} else if rv.Kind() == reflect.Map && !rv.IsNil() {
		// for each form value add it to the map
		for k, v := range form {
			if len(v) > 0 {
				rv.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v[0]))
			}
		}
	} else {
		return fmt.Errorf("v must point to a struct or a non-nil map type")
	}
	return nil
}

func unmarshalField(
	form url.Values,
	t reflect.StructField,
	v reflect.Value,
) error {

	// form field value
	fvs := form[t.Name]
	if len(fvs) == 0 {
		return nil
	}
	fv := fvs[0]
	// string -> type conversion
	switch v.Kind() {
	case reflect.Int64:
		// convert to Int64
		if i, err := strconv.ParseInt(fv, 10, 64); err == nil {
			v.SetInt(i)
		}
	case reflect.Int:
		// convert to Int
		// convert to Int64
		if i, err := strconv.ParseInt(fv, 10, 64); err == nil {
			v.SetInt(i)
		}
	case reflect.String:
		// copy string
		v.SetString(fv)
	case reflect.Bool:
		// the following strings convert to true
		// 1,true,on,yes
		if fv == "1" || fv == "true" || fv == "on" || fv == "yes" {
			v.SetBool(true)
		}
	case reflect.Slice:
		// ONLY STRING SLICES SO FAR
		// add all form values to slice
		sv := reflect.MakeSlice(t.Type, len(fvs), len(fvs))
		for i, fv := range fvs {
			svv := sv.Index(i)
			svv.SetString(fv)
		}
		v.Set(sv)
	default:
		fmt.Println("unknown type", v.Kind())
	}
	return nil
}

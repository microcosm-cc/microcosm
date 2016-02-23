package server

import (
	"net/http"

	"github.com/microcosm-cc/microcosm/controller"
)

var (
	rootHandlers = map[string]func(http.ResponseWriter, *http.Request){
		"/api/v1/auth": controller.AuthHandler,

		"/api/v1/hosts/{host:[0-9a-zA-Z-.]+}": controller.SiteHostHandler,

		"/api/v1/legal":                    controller.LegalsHandler,
		"/api/v1/legal/{document:service}": controller.LegalHandler,

		"/api/v1/metrics": controller.MetricsHandler,

		"/api/v1/site":                          controller.SiteHandler,
		"/api/v1/sites/{site_id:[0-9]+}":        controller.SiteHandler,
		"/api/v1/sites":                         controller.SitesHandler,
		"/api/v1/sites/{site_id:[0-9]+}/menu":   controller.MenuHandler,
		"/api/v1/sites/{site_id:[0-9]+}/status": controller.SiteCheckHandler,

		"/out/{short_url:[2-9a-zA-Z]+}": controller.RedirectHandler,

		"/api/v1/{type:profiles}/{profile_id:[0-9]+}":                                            controller.ProfileHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attachments":                                controller.AttachmentsHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attachments/{fileHash:[0-9A-Za-z]+}.{null}": controller.AttachmentHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attachments/{fileHash:[0-9A-Za-z]+}":        controller.AttachmentHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attributes":                                 controller.AttributesHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}":            controller.AttributeHandler,

		"/api/v1/reserved/{subdomain:[0-9a-zA-Z]+}": controller.SiteReservedHandler,

		"/api/v1/whoami": controller.WhoAmIHandler,
	}
	siteHandlers = map[string]func(http.ResponseWriter, *http.Request){
		"/":            controller.RootHandler,
		"/api":         controller.APIHandler,
		"/api/v1":      controller.V1Handler,
		"/api/v1/auth": controller.AuthHandler,

		"/api/v1/{type:comments}":                                                                controller.CommentsHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}":                                            controller.CommentHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}/attachments":                                controller.AttachmentsHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}/attachments/{fileHash:[0-9A-Za-z]+}.{null}": controller.AttachmentHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}/attachments/{fileHash:[0-9A-Za-z]+}":        controller.AttachmentHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}/incontext":                                  controller.CommentContextHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}/attributes":                                 controller.AttributesHandler,
		"/api/v1/{type:comments}/{comment_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}":            controller.AttributeHandler,

		"/api/v1/{type:conversations}":                                                          controller.ConversationsHandler,
		"/api/v1/{type:conversations}/{conversation_id:[0-9]+}":                                 controller.ConversationHandler,
		"/api/v1/{type:conversations}/{conversation_id:[0-9]+}/attributes":                      controller.AttributesHandler,
		"/api/v1/{type:conversations}/{conversation_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}": controller.AttributeHandler,
		"/api/v1/{type:conversations}/{conversation_id:[0-9]+}/lastcomment":                     controller.LastCommentHandler,
		"/api/v1/{type:conversations}/{conversation_id:[0-9]+}/newcomment":                      controller.NewCommentHandler,

		"/api/v1/{type:events}":                                                   controller.EventsHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}":                                 controller.EventHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/attendees":                       controller.AttendeesHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/attendees/{profile_id:[0-9]+}":   controller.AttendeeHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/attendeescsv":                    controller.AttendeesCSVHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/attributes":                      controller.AttributesHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}": controller.AttributeHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/lastcomment":                     controller.LastCommentHandler,
		"/api/v1/{type:events}/{event_id:[0-9]+}/newcomment":                      controller.NewCommentHandler,

		"/api/v1/files":                                controller.FilesHandler,
		"/api/v1/files/{fileHash:[0-9A-Za-z]+}.{null}": controller.FileHandler,
		"/api/v1/files/{fileHash:[0-9A-Za-z]+}":        controller.FileHandler,

		"/api/v1/geocode": controller.GeoCodeHandler,

		"/api/v1/{type:huddles}":                                                     controller.HuddlesHandler,
		"/api/v1/{type:huddles}/{huddle_id:[0-9]+}":                                  controller.HuddleHandler,
		"/api/v1/{type:huddles}/{huddle_id:[0-9]+}/lastcomment":                      controller.LastCommentHandler,
		"/api/v1/{type:huddles}/{huddle_id:[0-9]+}/newcomment":                       controller.NewCommentHandler,
		"/api/v1/{type:huddles}/{huddle_id:[0-9]+}/participants":                     controller.HuddleParticipantsHandler,
		"/api/v1/{type:huddles}/{huddle_id:[0-9]+}/participants/{profile_id:[0-9]+}": controller.HuddleParticipantHandler,

		"/api/v1/ignored": controller.IgnoredHandler,

		"/api/v1/legal":                    controller.LegalsHandler,
		"/api/v1/legal/{document:cookies}": controller.LegalHandler,
		"/api/v1/legal/{document:privacy}": controller.LegalHandler,
		"/api/v1/legal/{document:terms}":   controller.LegalHandler,

		"/api/v1/{type:microcosms}":                                                                             controller.MicrocosmsHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}":                                                       controller.MicrocosmHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/attributes":                                            controller.AttributesHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}":                       controller.AttributeHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles":                                                 controller.RolesHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles/{role_id:[0-9a-zA-Z_-]+}":                        controller.RoleHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles/{role_id:[0-9]+}/profiles":                       controller.RoleProfilesHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles/{role_id:[0-9]+}/profiles/{profile_id:[0-9]+}":   controller.RoleProfileHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles/{role_id:[0-9]+}/criteria":                       controller.RoleCriteriaHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles/{role_id:[0-9]+}/criteria/{criterion_id:[0-9]+}": controller.RoleCriterionHandler,
		"/api/v1/{type:microcosms}/{microcosm_id:[0-9]+}/roles/{role_id:[0-9]+}/members":                        controller.RoleMembersHandler,
		"/api/v1/{type:microcosms}/tree":                                                                        controller.MicrocosmsTreeHandler,

		"/api/v1/out/{short_url:[2-9a-zA-Z]+}": controller.RedirectHandler,

		"/api/v1/permission": controller.PermissionHandler,

		"/api/v1/{type:polls}":                                                  controller.PollsHandler,
		"/api/v1/{type:polls}/{poll_id:[0-9]+}":                                 controller.PollHandler,
		"/api/v1/{type:polls}/{poll_id:[0-9]+}/lastcomment":                     controller.LastCommentHandler,
		"/api/v1/{type:polls}/{poll_id:[0-9]+}/newcomment":                      controller.NewCommentHandler,
		"/api/v1/{type:polls}/{poll_id:[0-9]+}/attributes":                      controller.AttributesHandler,
		"/api/v1/{type:polls}/{poll_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}": controller.AttributeHandler,

		"/api/v1/{type:profiles}":                                                                controller.ProfilesHandler,
		"/api/v1/{type:profiles}/options":                                                        controller.ProfileOptionsHandler,
		"/api/v1/{type:profiles}/read":                                                           controller.ProfileReadHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}":                                            controller.ProfileHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attachments":                                controller.AttachmentsHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attachments/{fileHash:[0-9A-Za-z]+}.{null}": controller.AttachmentHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attachments/{fileHash:[0-9A-Za-z]+}":        controller.AttachmentHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attributes":                                 controller.AttributesHandler,
		"/api/v1/{type:profiles}/{profile_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}":            controller.AttributeHandler,

		"/api/v1/{type:questions}":                                                      controller.QuestionsHandler,
		"/api/v1/{type:questions}/{question_id:[0-9]+}":                                 controller.QuestionHandler,
		"/api/v1/{type:questions}/{question_id:[0-9]+}/attributes":                      controller.AttributesHandler,
		"/api/v1/{type:questions}/{question_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}": controller.AttributeHandler,
		"/api/v1/{type:questions}/{question_id:[0-9]+}/lastcomment":                     controller.LastCommentHandler,
		"/api/v1/{type:questions}/{question_id:[0-9]+}/newcomment":                      controller.NewCommentHandler,

		"/api/v1/resolve": controller.Redirect404Handler,

		"/api/v1/roles":                                                 controller.RolesHandler,
		"/api/v1/roles/{role_id:[0-9]+}":                                controller.RoleHandler,
		"/api/v1/roles/{role_id:[0-9]+}/profiles":                       controller.RoleProfilesHandler,
		"/api/v1/roles/{role_id:[0-9]+}/profiles/{profile_id:[0-9]+}":   controller.RoleProfileHandler,
		"/api/v1/roles/{role_id:[0-9]+}/criteria":                       controller.RoleCriteriaHandler,
		"/api/v1/roles/{role_id:[0-9]+}/criteria/{criterion_id:[0-9]+}": controller.RoleCriterionHandler,
		"/api/v1/roles/{role_id:[0-9]+}/members":                        controller.RoleMembersHandler,

		"/api/v1/search": controller.SearchHandler,

		"/api/v1/{type:site}":                                                  controller.SiteHandler,
		"/api/v1/{type:site}/menu":                                             controller.MenuHandler,
		"/api/v1/{type:site}/{site_id:[0-9]+}/attributes":                      controller.AttributesHandler,
		"/api/v1/{type:site}/{site_id:[0-9]+}/attributes/{key:[0-9a-zA-Z_-]+}": controller.AttributeHandler,

		"/api/v1/trending": controller.TrendingHandler,

		"/api/v1/updates":                                     controller.UpdatesHandler,
		"/api/v1/updates/preferences":                         controller.UpdateOptionsHandler,
		"/api/v1/updates/preferences/{update_type_id:[0-9]+}": controller.UpdateOptionHandler,

		"/api/v1/users":                  controller.UsersHandler,
		"/api/v1/users/{user_id:[0-9]+}": controller.UserHandler,

		"/api/v1/watchers":                     controller.WatchersHandler,
		"/api/v1/watchers/{watcher_id:[0-9]+}": controller.WatcherHandler,
		"/api/v1/watchers/delete":              controller.WatcherHandler,
		"/api/v1/watchers/patch":               controller.WatcherHandler,

		"/api/v1/whoami": controller.WhoAmIHandler,
	}
)

package models

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"

	h "github.com/microcosm-cc/microcosm/helpers"
)

// EmailMergeData encapsulates the merge object that will be used by the
// template merge to make values available to the template
type EmailMergeData struct {
	SiteTitle    string
	ProtoAndHost string
	ForProfile   ProfileSummaryType
	ForEmail     string
	ByProfile    ProfileSummaryType
	Subject      string
	ContextLink  string
	ContextText  string
	Body         string
}

// The only public interfaces to this dispatcher are the following methods
// which provide one interface per UpdateType.
//
// These methods then work out who should be notified of the update and how
// and will then call the notification methods

// SendUpdatesForNewCommentInItem is Update Type #1 : New comment in an item
// you're watching
func SendUpdatesForNewCommentInItem(
	siteID int64,
	comment CommentSummaryType,
) (
	int,
	error,
) {

	updateType, status, err := GetUpdateType(
		h.UpdateTypes[h.UpdateTypeNewComment],
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateType()", err)
		return status, err
	}

	// WHO GETS THE UPDATES?

	// Nobody watchers a comment, so we need to get the recipients for the item
	// the comment is attached to
	recipients, status, err := GetUpdateRecipients(
		siteID,
		comment.ItemTypeID,
		comment.ItemID,
		updateType.ID,
		comment.Meta.CreatedByID,
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateRecipients()", err)
		return status, err
	}

	// SEND UPDATES
	//
	// Freely acknowledging that we're going to loop the same thing many
	// times. But... we want all of the local updates to work even if the
	// emails fail, etc. So we'll do them primarily in the order of
	// delivery type rather than the order of recipients.

	if len(recipients) == 0 {
		glog.Info("No recipients to send updates to")
		return http.StatusOK, nil
	}

	///////////////////
	// LOCAL UPDATES //
	///////////////////
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("%s %+v", "h.GetTransaction()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Could not start transaction: %v", err.Error())
	}
	defer tx.Rollback()

	//glog.Info("Creating updates")
	sendEmails := false
	for _, recipient := range recipients {

		if !sendEmails &&
			recipient.SendEmail &&
			recipient.ForProfile.ID != comment.Meta.CreatedByID {

			sendEmails = true
		}

		// Everyone gets an update
		var update = UpdateType{}
		update.SiteID = siteID
		update.UpdateTypeID = updateType.ID
		update.ForProfileID = recipient.ForProfile.ID
		update.ItemTypeID = h.ItemTypes[h.ItemTypeComment]
		update.ItemID = comment.ID
		update.Meta.CreatedByID = comment.Meta.CreatedByID
		status, err := update.insert(tx)
		if err != nil {
			glog.Errorf("%s %+v", "update.insert(tx)", err)
			return status, err
		}
	}
	err = tx.Commit()
	if err != nil {
		glog.Errorf("%s %+v", "tx.Commit()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}
	//glog.Info("Updates sent")

	///////////////////
	// EMAIL UPDATES //
	///////////////////

	// For which we have a template in the form of a subject, HTML and text
	// body and we need to build up a data object to merge into the template
	if sendEmails {

		//glog.Info("Building email merge data")
		mergeData := EmailMergeData{}

		site, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("%s %+v", "GetSite()", err)
			return status, err
		}
		mergeData.SiteTitle = site.Title
		mergeData.ProtoAndHost = site.GetURL()

		mergeData.ContextLink = fmt.Sprintf(
			"%s/comments/%d/incontext/",
			mergeData.ProtoAndHost,
			comment.ID,
		)

		itemTitle, status, err := GetTitle(
			siteID,
			comment.ItemTypeID,
			comment.ItemID,
			0,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetTitle()", err)
			return status, err
		}
		mergeData.ContextText = itemTitle

		byProfile, status, err := GetProfileSummary(
			siteID,
			comment.Meta.CreatedByID,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}
		mergeData.ByProfile = byProfile

		mergeData.Body = comment.HTML

		// And the templates
		subjectTemplate, textTemplate, htmlTemplate, status, err :=
			updateType.GetEmailTemplates()
		if err != nil {
			glog.Errorf("%s %+v", "updateType.GetEmailTemplates()", err)
			return status, err
		}

		for _, recipient := range recipients {
			// Everyone who wants an email gets an email... except:
			// 1) the author
			// 2) anyone already emailed
			// 3) if this comment is a reply to another, the parent comment
			//    author as that is handled by the NewReplyToYourComment thing
			lastRead, status, err := GetLastReadTime(
				comment.ItemTypeID,
				comment.ItemID,
				recipient.ForProfile.ID,
			)
			if err != nil {
				glog.Errorf("%s %+v", "GetLastReadTime()", err)
				return status, err
			}

			var parentCommentCreatedByID int64
			if comment.InReplyTo > 0 {

				parentComment, status, err := GetCommentSummary(
					siteID,
					comment.InReplyTo,
				)
				if err != nil {
					glog.Errorf("%s %+v", "GetComment()", err)
					return status, err
				}
				parentCommentCreatedByID = parentComment.Meta.CreatedByID
			}

			if recipient.SendEmail &&
				recipient.ForProfile.ID != comment.Meta.CreatedByID &&
				(lastRead.After(recipient.LastNotified) ||
					recipient.LastNotified.IsZero()) &&
				recipient.ForProfile.ID != parentCommentCreatedByID {

				// Personalisation of email
				mergeData.ForProfile = recipient.ForProfile

				user, status, err := GetUser(recipient.ForProfile.UserID)
				if err != nil {
					glog.Errorf("%s %+v", "GetUser()", err)
					return status, err
				}
				mergeData.ForEmail = user.Email

				status, err = MergeAndSendEmail(
					siteID,
					fmt.Sprintf(emailFrom, GetSiteTitle(siteID)),
					mergeData.ForEmail,
					subjectTemplate,
					textTemplate,
					htmlTemplate,
					mergeData,
				)
				if err != nil {
					glog.Errorf("%s %+v", "MergeAndSendEmail()", err)
				}

				recipient.Watcher.UpdateLastNotified()
			}
		}
	}

	/////////////////
	// SMS UPDATES //
	/////////////////
	for _, recipient := range recipients {
		// Everyone who wants an SMS, except the author, gets an SMS
		if recipient.SendSMS {
			// Send SMS
		}
	}

	return http.StatusOK, nil
}

// SendUpdatesForNewReplyToYourComment is Update Type #2 : New reply to a
// comment that you made
func SendUpdatesForNewReplyToYourComment(
	siteID int64,
	comment CommentSummaryType,
) (
	int,
	error,
) {

	updateType, status, err := GetUpdateType(
		h.UpdateTypes[h.UpdateTypeReplyToComment],
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateType()", err)
		return status, err
	}

	parentComment, status, err := GetCommentSummary(siteID, comment.InReplyTo)
	if err != nil {
		glog.Errorf("%s %+v", "GetComment()", err)
		return status, err
	}
	profileID := parentComment.Meta.CreatedByID

	forProfile, status, err := GetProfileSummary(siteID, profileID)
	if err != nil {
		glog.Errorf("%s %+v", "GetProfileSummary()", err)
		return http.StatusInternalServerError, err
	}

	///////////////////
	// LOCAL UPDATES //
	///////////////////
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("%s %+v", "h.GetTransaction()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Could not start transaction: %v", err.Error())
	}
	defer tx.Rollback()

	//glog.Info("Creating update")

	// Everyone gets an update
	var update = UpdateType{}
	update.SiteID = siteID
	update.UpdateTypeID = updateType.ID
	update.ForProfileID = forProfile.ID
	update.ItemTypeID = h.ItemTypes[h.ItemTypeComment]
	update.ItemID = comment.ID
	update.Meta.CreatedByID = comment.Meta.CreatedByID
	status, err = update.insert(tx)
	if err != nil {
		glog.Errorf("%s %+v", "update.insert(tx)", err)
		return status, err
	}

	err = tx.Commit()
	if err != nil {
		glog.Errorf("%s %+v", "tx.Commit()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	//glog.Info("Update sent")

	///////////////////
	// EMAIL UPDATES //
	///////////////////
	updateOptions, status, err := GetCommunicationOptions(
		siteID,
		profileID,
		updateType.ID,
		h.ItemTypes[h.ItemTypeComment],
		comment.ID,
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateOptionForUpdateType()", err)
		return status, err
	}

	if updateOptions.SendEmail {

		glog.Info("Building email merge data")
		mergeData := EmailMergeData{}

		site, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("%s %+v", "GetSite()", err)
			return status, err
		}
		mergeData.SiteTitle = site.Title
		mergeData.ProtoAndHost = site.GetURL()

		mergeData.ContextLink = fmt.Sprintf(
			"%s/comments/%d/incontext/",
			mergeData.ProtoAndHost,
			comment.ID,
		)

		itemTitle, status, err := GetTitle(
			siteID,
			comment.ItemTypeID,
			comment.ItemID,
			0,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetTitle()", err)
			return status, err
		}
		mergeData.ContextText = itemTitle

		byProfile, status, err := GetProfileSummary(
			siteID,
			comment.Meta.CreatedByID,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}
		mergeData.ByProfile = byProfile

		mergeData.Body = comment.HTML

		// And the templates
		subjectTemplate, textTemplate, htmlTemplate, status, err :=
			updateType.GetEmailTemplates()
		if err != nil {
			glog.Errorf("%s %+v", "updateType.GetEmailTemplates()", err)
			return status, err
		}

		// Personalisation of email
		mergeData.ForProfile = forProfile

		user, status, err := GetUser(forProfile.UserID)
		if err != nil {
			glog.Errorf("%s %+v", "GetUser()", err)
			return status, err
		}
		mergeData.ForEmail = user.Email

		status, err = MergeAndSendEmail(
			siteID,
			fmt.Sprintf(emailFrom, GetSiteTitle(siteID)),
			mergeData.ForEmail,
			subjectTemplate,
			textTemplate,
			htmlTemplate,
			mergeData,
		)
		if err != nil {
			glog.Errorf("%s %+v", "MergeAndSendEmail()", err)
		}
	}

	/////////////////
	// SMS UPDATES //
	/////////////////
	if updateOptions.SendSMS {
	}

	return http.StatusOK, nil
}

// SendUpdatesForNewMentionInComment is Update Type #3 : New mention in a comment
func SendUpdatesForNewMentionInComment(
	siteID int64,
	forProfileID int64,
	commentID int64,
) (
	int,
	error,
) {

	updateType, status, err := GetUpdateType(
		h.UpdateTypes[h.UpdateTypeMentioned],
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateType()", err)
		return status, err
	}

	// Two things to note about this method and where it was called from...
	// 1) Update.upsert() has already created the entry in the updates table
	// 2) The revision of the comment, and maybe the comment itself, may not yet
	//    exist
	//
	// This is because this is called mid-transaction within mentions.go
	//
	// It's our job to send the email and/or SMS, but we need to sleep so that
	// the transaction finishes and everything is ready
	time.Sleep(5 * time.Second)

	updateOptions, status, err := GetCommunicationOptions(
		siteID,
		forProfileID,
		updateType.ID,
		h.ItemTypes[h.ItemTypeComment],
		commentID,
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateOptionForUpdateType()", err)
		return status, err
	}

	///////////////////
	// EMAIL UPDATES //
	///////////////////
	if updateOptions.SendEmail {
		comment, status, err := GetCommentSummary(siteID, commentID)
		if err != nil {
			glog.Errorf("%s %+v", "GetComment()", err)
			return status, err
		}

		forProfile, status, err := GetProfileSummary(siteID, forProfileID)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}

		glog.Info("Building email merge data")
		mergeData := EmailMergeData{}

		site, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("%s %+v", "GetSite()", err)
			return status, err
		}
		mergeData.SiteTitle = site.Title
		mergeData.ProtoAndHost = site.GetURL()

		mergeData.ContextLink = fmt.Sprintf(
			"%s/comments/%d/incontext/",
			mergeData.ProtoAndHost,
			comment.ID,
		)

		itemTitle, status, err := GetTitle(
			siteID,
			comment.ItemTypeID,
			comment.ItemID,
			0,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetTitle()", err)
			return status, err
		}
		mergeData.ContextText = itemTitle

		byProfile, status, err := GetProfileSummary(
			siteID,
			comment.Meta.CreatedByID,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}
		mergeData.ByProfile = byProfile

		mergeData.Body = comment.HTML

		// And the templates
		subjectTemplate, textTemplate, htmlTemplate, status, err :=
			updateType.GetEmailTemplates()
		if err != nil {
			glog.Errorf("%s %+v", "updateType.GetEmailTemplates()", err)
			return status, err
		}

		// Personalisation of email
		mergeData.ForProfile = forProfile

		user, status, err := GetUser(forProfile.UserID)
		if err != nil {
			glog.Errorf("%s %+v", "GetUser()", err)
			return status, err
		}
		mergeData.ForEmail = user.Email

		status, err = MergeAndSendEmail(
			siteID,
			fmt.Sprintf(emailFrom, GetSiteTitle(siteID)),
			mergeData.ForEmail,
			subjectTemplate,
			textTemplate,
			htmlTemplate,
			mergeData,
		)
		if err != nil {
			glog.Errorf("%s %+v", "MergeAndSendEmail()", err)
		}

	}

	/////////////////
	// SMS UPDATES //
	/////////////////
	if updateOptions.SendSMS {

	}

	return http.StatusOK, nil
}

// SendUpdatesForNewCommentInHuddle is Update Type #4 : New comment in a huddle
// you are participating in
//
// TODO(buro9): This is still based on the same code as new comment, but are
// there some special rules because it's a huddle? If so, they're not yet
// implemented.
func SendUpdatesForNewCommentInHuddle(
	siteID int64,
	comment CommentSummaryType,
) (
	int,
	error,
) {

	updateType, status, err := GetUpdateType(
		h.UpdateTypes[h.UpdateTypeNewCommentInHuddle],
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateType()", err)
		return status, err
	}

	// WHO GETS THE UPDATES?

	// Nobody watchers a comment, so we need to get the recipients for the item
	// the comment is attached to
	recipients, status, err := GetUpdateRecipients(
		siteID,
		comment.ItemTypeID,
		comment.ItemID,
		updateType.ID,
		comment.Meta.CreatedByID,
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateRecipients()", err)
		return status, err
	}

	// SEND UPDATES
	//
	// Freely acknowledging that we're going to loop the same thing many
	// times. But... we want all of the local updates to work even if the
	// emails fail, etc. So we'll do them primarily in the order of
	// delivery type rather than the order of recipients.

	if len(recipients) == 0 {
		glog.Info("No recipients to send updates to")
		return http.StatusOK, nil
	}

	///////////////////
	// LOCAL UPDATES //
	///////////////////
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("%s %+v", "h.GetTransaction()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Could not start transaction: %v", err.Error())
	}
	defer tx.Rollback()

	//glog.Info("Creating updates")
	sendEmails := false
	for _, recipient := range recipients {

		if !sendEmails &&
			recipient.SendEmail &&
			recipient.ForProfile.ID != comment.Meta.CreatedByID {

			sendEmails = true
		}

		// Everyone gets an update
		var update = UpdateType{}
		update.SiteID = siteID
		update.UpdateTypeID = updateType.ID
		update.ForProfileID = recipient.ForProfile.ID
		update.ItemTypeID = h.ItemTypes[h.ItemTypeComment]
		update.ItemID = comment.ID
		update.Meta.CreatedByID = comment.Meta.CreatedByID
		status, err := update.insert(tx)
		if err != nil {
			glog.Errorf("%s %+v", "update.insert(tx)", err)
			return status, err
		}
	}
	err = tx.Commit()
	if err != nil {
		glog.Errorf("%s %+v", "tx.Commit()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	//glog.Info("Updates sent")

	///////////////////
	// EMAIL UPDATES //
	///////////////////

	// For which we have a template in the form of a subject, HTML and text
	// body and we need to build up a data object to merge into the template
	if sendEmails {

		glog.Info("Building email merge data")
		mergeData := EmailMergeData{}

		site, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("%s %+v", "GetSite()", err)
			return status, err
		}
		mergeData.SiteTitle = site.Title
		mergeData.ProtoAndHost = site.GetURL()

		mergeData.ContextLink = fmt.Sprintf(
			"%s/comments/%d/incontext/",
			mergeData.ProtoAndHost,
			comment.ID,
		)

		itemTitle, status, err := GetTitle(
			siteID,
			comment.ItemTypeID,
			comment.ItemID,
			0,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetTitle()", err)
			return status, err
		}
		mergeData.ContextText = itemTitle

		byProfile, status, err := GetProfileSummary(
			siteID,
			comment.Meta.CreatedByID,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}
		mergeData.ByProfile = byProfile

		mergeData.Body = comment.HTML

		// And the templates
		subjectTemplate, textTemplate, htmlTemplate, status, err :=
			updateType.GetEmailTemplates()
		if err != nil {
			glog.Errorf("%s %+v", "updateType.GetEmailTemplates()", err)
			return status, err
		}

		for _, recipient := range recipients {
			// Everyone who wants an email gets an email... except:
			// 1) the author
			// 2) anyone already emailed
			// 3) if this comment is a reply to another, the parent comment
			//    author as that is handled by the NewReplyToYourComment thing
			lastRead, status, err := GetLastReadTime(
				comment.ItemTypeID,
				comment.ItemID,
				recipient.ForProfile.ID,
			)
			if err != nil {
				glog.Errorf("%s %+v", "GetLastReadTime()", err)
				return status, err
			}

			var parentCommentCreatedByID int64
			if comment.InReplyTo > 0 {

				parentComment, status, err := GetCommentSummary(
					siteID,
					comment.InReplyTo,
				)
				if err != nil {
					glog.Errorf("%s %+v", "GetComment()", err)
					return status, err
				}
				parentCommentCreatedByID = parentComment.Meta.CreatedByID
			}

			if recipient.SendEmail &&
				recipient.ForProfile.ID != comment.Meta.CreatedByID &&
				(lastRead.After(recipient.LastNotified) ||
					recipient.LastNotified.IsZero()) &&
				recipient.ForProfile.ID != parentCommentCreatedByID {

				// Personalisation of email
				mergeData.ForProfile = recipient.ForProfile

				user, status, err := GetUser(recipient.ForProfile.UserID)
				if err != nil {
					glog.Errorf("%s %+v", "GetUser()", err)
					return status, err
				}
				mergeData.ForEmail = user.Email

				status, err = MergeAndSendEmail(
					siteID,
					fmt.Sprintf(emailFrom, GetSiteTitle(siteID)),
					mergeData.ForEmail,
					subjectTemplate,
					textTemplate,
					htmlTemplate,
					mergeData,
				)
				if err != nil {
					glog.Errorf("%s %+v", "MergeAndSendEmail()", err)
				}

				recipient.Watcher.UpdateLastNotified()
			}
		}
	}

	/////////////////
	// SMS UPDATES //
	/////////////////
	for _, recipient := range recipients {
		// Everyone who wants an SMS, except the author, gets an SMS
		if recipient.SendSMS {
			// Send SMS
		}
	}

	return http.StatusOK, nil
}

// SendUpdatesForNewAttendeeInAnEvent is Update Type #5 : New attendee in an
// event
func SendUpdatesForNewAttendeeInAnEvent(
	siteID int64,
	attendee AttendeeType,
) (
	int,
	error,
) {

	updateType, status, err := GetUpdateType(
		h.UpdateTypes[h.UpdateTypeNewEventAttendee],
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateType()", err)
		return status, err
	}

	// WHO GETS THE UPDATES?

	// Nobody watchers a comment, so we need to get the recipients for the item
	// the comment is attached to
	recipients, status, err := GetUpdateRecipients(
		siteID,
		h.ItemTypes[h.ItemTypeEvent],
		attendee.EventID,
		updateType.ID,
		attendee.ProfileID,
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateRecipients()", err)
		return status, err
	}

	// SEND UPDATES
	//
	// Freely acknowledging that we're going to loop the same thing many
	// times. But... we want all of the local updates to work even if the
	// emails fail, etc. So we'll do them primarily in the order of
	// delivery type rather than the order of recipients.

	if len(recipients) == 0 {
		glog.Info("No recipients to send updates to")
		return http.StatusOK, nil
	}

	///////////////////
	// LOCAL UPDATES //
	///////////////////
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("%s %+v", "h.GetTransaction()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Could not start transaction: %v", err.Error())
	}
	defer tx.Rollback()

	glog.Info("Creating updates")
	sendEmails := false
	for _, recipient := range recipients {

		if !sendEmails &&
			recipient.SendEmail &&
			recipient.ForProfile.ID != attendee.ProfileID {

			sendEmails = true
		}

		// Everyone gets an update
		var update = UpdateType{}
		update.SiteID = siteID
		update.UpdateTypeID = updateType.ID
		update.ForProfileID = recipient.ForProfile.ID
		update.ItemTypeID = h.ItemTypes[h.ItemTypeEvent]
		update.ItemID = attendee.EventID
		update.Meta.CreatedByID = attendee.ProfileID
		status, err := update.insert(tx)
		if err != nil {
			glog.Errorf("%s %+v", "update.insert(tx)", err)
			return status, err
		}
	}
	err = tx.Commit()
	if err != nil {
		glog.Errorf("%s %+v", "tx.Commit()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	//	glog.Info("Updates sent")

	///////////////////
	// EMAIL UPDATES //
	///////////////////

	// For which we have a template in the form of a subject, HTML and text
	// body and we need to build up a data object to merge into the template
	if sendEmails {

		glog.Info("Building email merge data")
		mergeData := EmailMergeData{}

		site, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("%s %+v", "GetSite()", err)
			return status, err
		}
		mergeData.SiteTitle = site.Title
		mergeData.ProtoAndHost = site.GetURL()

		mergeData.ContextLink = fmt.Sprintf(
			"%s/events/%d/",
			mergeData.ProtoAndHost,
			attendee.EventID,
		)

		itemTitle, status, err := GetTitle(
			siteID,
			h.ItemTypes[h.ItemTypeEvent],
			attendee.EventID,
			0,
		)
		if err != nil {
			glog.Errorf("%s %+v", "GetTitle()", err)
			return status, err
		}
		mergeData.ContextText = itemTitle

		byProfile, status, err := GetProfileSummary(siteID, attendee.ProfileID)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}
		mergeData.ByProfile = byProfile

		// And the templates
		subjectTemplate, textTemplate, htmlTemplate, status, err :=
			updateType.GetEmailTemplates()
		if err != nil {
			glog.Errorf("%s %+v", "updateType.GetEmailTemplates()", err)
			return status, err
		}

		for _, recipient := range recipients {
			// Everyone who wants an email gets an email... except:
			// 1) the author
			// 2) anyone already emailed
			// 3) if this comment is a reply to another, the parent comment author
			//    as that is handled by the NewReplyToYourComment thing
			lastRead, status, err := GetLastReadTime(
				h.ItemTypes[h.ItemTypeEvent],
				attendee.EventID,
				recipient.ForProfile.ID,
			)
			if err != nil {
				glog.Errorf("%s %+v", "GetLastReadTime()", err)
				return status, err
			}

			if recipient.SendEmail &&
				recipient.ForProfile.ID != attendee.ProfileID &&
				(lastRead.After(recipient.LastNotified) ||
					recipient.LastNotified.IsZero()) {

				// Personalisation of email
				mergeData.ForProfile = recipient.ForProfile

				user, status, err := GetUser(recipient.ForProfile.UserID)
				if err != nil {
					glog.Errorf("%s %+v", "GetUser()", err)
					return status, err
				}
				mergeData.ForEmail = user.Email

				status, err = MergeAndSendEmail(
					siteID,
					fmt.Sprintf(emailFrom, GetSiteTitle(siteID)),
					mergeData.ForEmail,
					subjectTemplate,
					textTemplate,
					htmlTemplate,
					mergeData,
				)
				if err != nil {
					glog.Errorf("%s %+v", "MergeAndSendEmail()", err)
				}

				recipient.Watcher.UpdateLastNotified()
			}
		}
	}

	/////////////////
	// SMS UPDATES //
	/////////////////
	for _, recipient := range recipients {
		// Everyone who wants an SMS, except the author, gets an SMS
		if recipient.SendSMS {
			// Send SMS
		}
	}

	return http.StatusOK, nil
}

// SendUpdatesForNewVoteInAPoll is Update Type #6 : New vote in a poll
func SendUpdatesForNewVoteInAPoll(siteID int64, poll *PollType) (int, error) {

	// TODO(buro9): Not yet implemented

	return http.StatusOK, nil
}

// SendUpdatesForEventReminder is Update Type #7 : Event reminder as event imminent
func SendUpdatesForEventReminder(siteID int64, event *EventType) (int, error) {

	// TODO(buro9): Not yet implemented but could be done

	return http.StatusOK, nil
}

// SendUpdatesForNewItemInAMicrocosm is Update Type #8 : A new item in a Microcosm
func SendUpdatesForNewItemInAMicrocosm(
	siteID int64,
	item interface{},
) (
	int,
	error,
) {

	updateType, status, err := GetUpdateType(h.UpdateTypes[h.UpdateTypeNewItem])
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateType()", err)
		return status, err
	}

	var (
		itemTypeID   int64
		itemType     string
		itemID       int64
		createdByID  int64
		conversation ConversationType
		event        EventType
		poll         PollType
		question     QuestionType
	)

	switch item.(type) {
	case ConversationType:
		conversation = item.(ConversationType)
		itemTypeID = h.ItemTypes[h.ItemTypeConversation]
		itemType = h.ItemTypeConversation
		itemID = conversation.ID
		createdByID = conversation.Meta.CreatedByID

	case EventType:
		event = item.(EventType)
		itemTypeID = h.ItemTypes[h.ItemTypeEvent]
		itemType = h.ItemTypeEvent
		itemID = event.ID
		createdByID = event.Meta.CreatedByID

	case PollType:
		poll = item.(PollType)
		itemTypeID = h.ItemTypes[h.ItemTypePoll]
		itemType = h.ItemTypePoll
		itemID = poll.ID
		createdByID = poll.Meta.CreatedByID

	case QuestionType:
		question = item.(QuestionType)
		itemTypeID = h.ItemTypes[h.ItemTypeQuestion]
		itemType = h.ItemTypeQuestion
		itemID = question.ID
		createdByID = question.Meta.CreatedByID

	default:
		glog.Errorf("%s %+v", "type not known", item)
		return http.StatusExpectationFailed,
			fmt.Errorf("Type of item is mysterious")
	}

	// WHO GETS THE UPDATES?
	recipients, status, err := GetUpdateRecipients(
		siteID,
		itemTypeID,
		itemID,
		updateType.ID,
		createdByID,
	)
	if err != nil {
		glog.Errorf("%s %+v", "GetUpdateRecipients()", err)
		return status, err
	}

	// SEND UPDATES
	if len(recipients) == 0 {
		glog.Info("No recipients to send updates to")
		return http.StatusOK, nil
	}

	///////////////////
	// LOCAL UPDATES //
	///////////////////
	tx, err := h.GetTransaction()
	if err != nil {
		glog.Errorf("%s %+v", "h.GetTransaction()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Could not start transaction: %v", err.Error())
	}
	defer tx.Rollback()

	glog.Info("Creating updates")
	sendEmails := false
	for _, recipient := range recipients {

		if !sendEmails &&
			recipient.SendEmail &&
			recipient.ForProfile.ID != createdByID {

			sendEmails = true
		}

		// Everyone gets an update
		var update = UpdateType{}
		update.SiteID = siteID
		update.UpdateTypeID = updateType.ID
		update.ForProfileID = recipient.ForProfile.ID
		update.ItemTypeID = itemTypeID
		update.ItemID = itemID
		update.Meta.CreatedByID = createdByID
		status, err := update.insert(tx)
		if err != nil {
			glog.Errorf("%s %+v", "update.insert(tx)", err)
			return status, err
		}
	}
	err = tx.Commit()
	if err != nil {
		glog.Errorf("%s %+v", "tx.Commit()", err)
		return http.StatusInternalServerError,
			fmt.Errorf("Transaction failed: %v", err.Error())
	}

	//	glog.Info("Updates sent")

	///////////////////
	// EMAIL UPDATES //
	///////////////////

	// For which we have a template in the form of a subject, HTML and text
	// body and we need to build up a data object to merge into the template
	if sendEmails {

		glog.Info("Building email merge data")
		mergeData := EmailMergeData{}

		site, status, err := GetSite(siteID)
		if err != nil {
			glog.Errorf("%s %+v", "GetSite()", err)
			return status, err
		}
		mergeData.SiteTitle = site.Title
		mergeData.ProtoAndHost = site.GetURL()

		mergeData.ContextLink = fmt.Sprintf(
			"%s/%ss/%d/",
			mergeData.ProtoAndHost,
			itemType,
			itemID,
		)

		itemTitle, status, err := GetTitle(siteID, itemTypeID, itemID, 0)
		if err != nil {
			glog.Errorf("%s %+v", "GetTitle()", err)
			return status, err
		}
		mergeData.ContextText = itemTitle

		byProfile, status, err := GetProfileSummary(siteID, createdByID)
		if err != nil {
			glog.Errorf("%s %+v", "GetProfileSummary()", err)
			return http.StatusInternalServerError, err
		}
		mergeData.ByProfile = byProfile

		// And the templates
		subjectTemplate, textTemplate, htmlTemplate, status, err :=
			updateType.GetEmailTemplates()
		if err != nil {
			glog.Errorf("%s %+v", "updateType.GetEmailTemplates()", err)
			return status, err
		}

		for _, recipient := range recipients {
			// Everyone who wants an email gets an email... except:
			// 1) the author
			if recipient.SendEmail &&
				recipient.ForProfile.ID != createdByID {

				// Personalisation of email
				mergeData.ForProfile = recipient.ForProfile

				user, status, err := GetUser(recipient.ForProfile.UserID)
				if err != nil {
					glog.Errorf("%s %+v", "GetUser()", err)
					return status, err
				}
				mergeData.ForEmail = user.Email

				status, err = MergeAndSendEmail(
					siteID,
					fmt.Sprintf(emailFrom, GetSiteTitle(siteID)),
					mergeData.ForEmail,
					subjectTemplate,
					textTemplate,
					htmlTemplate,
					mergeData,
				)
				if err != nil {
					glog.Errorf("%s %+v", "MergeAndSendEmail()", err)
				}

				recipient.Watcher.UpdateLastNotified()
			}
		}
	}

	/////////////////
	// SMS UPDATES //
	/////////////////
	for _, recipient := range recipients {
		// Everyone who wants an SMS, except the author, gets an SMS
		if recipient.SendSMS {
			// Send SMS
		}
	}

	return http.StatusOK, nil
}

// SendUpdatesForNewProfileOnSite is Update Type #9 : A new item in a Microcosm
func SendUpdatesForNewProfileOnSite(
	siteID int64,
	profileID int64,
) (
	int,
	error,
) {

	// Also send to watchers of the site

	// TODO(buro9): Not yet implemented but could be done

	return http.StatusOK, nil
}

// GetCommunicationOptions returns a user's update options if present,
// otherwise it returns the default preference for the given update type.
func GetCommunicationOptions(
	siteID int64,
	profileID int64,
	updateTypeID int64,
	itemTypeID int64,
	itemID int64,
) (
	UpdateOptionType,
	int,
	error,
) {

	_, status, err := GetProfileOptions(profileID)
	if err != nil {
		glog.Errorf("GetProfileOptions(%d) %+v", profileID, err)
		// Can't do anything here as the profile_id fkey constraint will fail
		return UpdateOptionType{}, status, fmt.Errorf("Insert of update options failed")
	}

	db, err := h.GetConnection()
	if err != nil {
		glog.Errorf("h.GetConnection() %+v", err)
		return UpdateOptionType{}, http.StatusInternalServerError, err
	}

	rows, err := db.Query(`
SELECT send_email
      ,send_sms
      ,description
  FROM get_communication_options($1, $2, $3, $4, $5)
       LEFT JOIN update_types a ON update_type_id = $5`,
		siteID,
		itemID,
		itemTypeID,
		profileID,
		updateTypeID,
	)
	if err != nil {
		glog.Errorf(
			"db.Query(%d, %d, %d, %d, %d) %+v",
			siteID,
			itemID,
			itemTypeID,
			profileID,
			updateTypeID,
			err,
		)
		return UpdateOptionType{},
			http.StatusInternalServerError,
			fmt.Errorf("Database query failed")
	}
	defer rows.Close()

	var m UpdateOptionType

	for rows.Next() {
		m = UpdateOptionType{}
		err = rows.Scan(
			&m.SendEmail,
			&m.SendSMS,
			&m.Description,
		)
		if err != nil {
			glog.Errorf("rows.Scan() %+v", err)
			return UpdateOptionType{},
				http.StatusInternalServerError,
				fmt.Errorf("Row parsing error")
		}
	}
	err = rows.Err()
	if err != nil {
		glog.Errorf("rows.Err() %+v", err)
		return UpdateOptionType{},
			http.StatusInternalServerError,
			fmt.Errorf("Error fetching rows")
	}
	rows.Close()

	m.ProfileID = int64(profileID)
	m.UpdateTypeID = int64(updateTypeID)

	return m, http.StatusOK, nil
}

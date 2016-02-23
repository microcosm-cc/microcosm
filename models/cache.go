package models

import (
	"fmt"

	"github.com/golang/glog"

	c "github.com/microcosm-cc/microcosm/cache"
	h "github.com/microcosm-cc/microcosm/helpers"
)

// This file contains setup and helper functions for caching
// model objects with memcache. It should only contain functions
// specifically for dealing with models; anything else should go in
// the cache package.

var (
	mcAccessTokenKeys = map[int]string{
		c.CacheDetail: "au_%s",
	}
	mcAttendeeKeys = map[int]string{
		c.CacheDetail: "at_d%d",
	}
	mcCommentKeys = map[int]string{
		c.CacheDetail: "cm_d%d",
	}
	mcConversationKeys = map[int]string{
		c.CacheDetail:     "cv_d%d",
		c.CacheSummary:    "cv_s%d",
		c.CacheItem:       "cv_i%d",
		c.CacheBreadcrumb: "cv_b%d",
	}
	mcEventKeys = map[int]string{
		c.CacheDetail:     "ev_d%d",
		c.CacheSummary:    "ev_s%d",
		c.CacheItem:       "ev_i%d",
		c.CacheProfileIds: "ev_l%d",
		c.CacheBreadcrumb: "ev_b%d",
	}
	mcHuddleKeys = map[int]string{
		c.CacheDetail:  "hd_d%d",
		c.CacheSummary: "hd_s%d",
		c.CacheItem:    "hd_i%d",
		c.CacheTitle:   "hd_t%d",
	}
	mcMicrocosmKeys = map[int]string{
		c.CacheDetail:     "ms_d%d",
		c.CacheSummary:    "ms_s%d",
		c.CacheTitle:      "ms_t%d",
		c.CacheBreadcrumb: "ms_b%d",
	}
	mcPollKeys = map[int]string{
		c.CacheDetail:     "po_d%d",
		c.CacheSummary:    "po_s%d",
		c.CacheItem:       "po_i%d",
		c.CacheBreadcrumb: "po_b%d",
	}
	mcProfileKeys = map[int]string{
		c.CacheDetail:  "pr_d%d",
		c.CacheSummary: "pr_s%d",
		c.CacheUser:    "us_d%d",
		c.CacheCounts:  "pr_c%d",
		c.CacheOptions: "pr_o%d",
	}
	mcQuestionKeys = map[int]string{
		c.CacheDetail:     "q_d%d",
		c.CacheSummary:    "q_s%d",
		c.CacheItem:       "q_i%d",
		c.CacheBreadcrumb: "q_b%d",
	}
	mcRoleKeys = map[int]string{
		c.CacheDetail: "r_d%d",
	}
	mcSiteKeys = map[int]string{
		c.CacheDetail:    "s_d%d",
		c.CacheDomain:    "s_do%s",
		c.CacheSubdomain: "s_sd%s",
		c.CacheTitle:     "s_t%d",
		c.CacheCounts:    "s_c%d",
		c.CacheRootID:    "s_r%d",
	}
	mcUpdateKeys = map[int]string{
		c.CacheDetail: "u_d%d",
	}
	mcUpdateTypeKeys = map[int]string{
		c.CacheDetail: "ut_d%d",
	}
	mcUpdateCountKeys = map[int]string{
		c.CacheDetail: "uc_d%d",
	}
	mcWatcherKeys = map[int]string{
		c.CacheDetail: "w_d%d",
	}
)

const mcTTL int32 = 60 * 60 * 24 * 7 // 1 Week

// PurgeCache removes an item from the cache
func PurgeCache(itemTypeID int64, itemID int64) {
	switch itemTypeID {

	case h.ItemTypes[h.ItemTypeAlbum]:

	case h.ItemTypes[h.ItemTypeArticle]:

	case h.ItemTypes[h.ItemTypeAttendee]:
		for _, mcKeyFmt := range mcAttendeeKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeClassified]:

	case h.ItemTypes[h.ItemTypeComment]:
		for _, mcKeyFmt := range mcCommentKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeConversation]:
		for _, mcKeyFmt := range mcConversationKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeEvent]:
		for _, mcKeyFmt := range mcEventKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeHuddle]:
		for _, mcKeyFmt := range mcHuddleKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeMicrocosm]:
		// Need to purge parents too but not the root.
		links, _, err := getMicrocosmParents(itemID)
		if err != nil {
			glog.Errorf("+%v", err)
			for _, mcKeyFmt := range mcMicrocosmKeys {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
			return
		}

		for _, link := range links {
			if link.Level == 1 {
				continue
			}
			for _, mcKeyFmt := range mcMicrocosmKeys {
				c.Delete(fmt.Sprintf(mcKeyFmt, link.ID))
			}
		}

	case h.ItemTypes[h.ItemTypePoll]:
		for _, mcKeyFmt := range mcPollKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeProfile]:
		for _, mcKeyFmt := range mcProfileKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeQuestion]:
		for _, mcKeyFmt := range mcQuestionKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeRole]:

		// Need to flush the database cache if we're flushing everything to
		// do with the role
		tx, err := h.GetTransaction()
		if err != nil {
			glog.Errorf("+%v", err)
			return
		}
		defer tx.Rollback()

		_, err = FlushRoleMembersCacheByRoleID(tx, itemID)
		if err != nil {
			glog.Errorf("+%v", err)
			return
		}

		err = tx.Commit()
		if err != nil {
			glog.Errorf("+%v", err)
			return
		}

		for _, mcKeyFmt := range mcRoleKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeSite]:
		for _, mcKeyFmt := range mcSiteKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeUpdate]:
		for _, mcKeyFmt := range mcUpdateKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	case h.ItemTypes[h.ItemTypeWatcher]:
		for _, mcKeyFmt := range mcWatcherKeys {
			c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
		}

	default:
	}
}

// PurgeCacheByScope removes part of an item from the cache
func PurgeCacheByScope(scope int, itemTypeID int64, itemID int64) {
	switch itemTypeID {

	case h.ItemTypes[h.ItemTypeAlbum]:

	case h.ItemTypes[h.ItemTypeArticle]:

	case h.ItemTypes[h.ItemTypeAttendee]:
		for mcKey, mcKeyFmt := range mcAttendeeKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeClassified]:

	case h.ItemTypes[h.ItemTypeComment]:
		for mcKey, mcKeyFmt := range mcCommentKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeConversation]:
		for mcKey, mcKeyFmt := range mcConversationKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeEvent]:
		for mcKey, mcKeyFmt := range mcEventKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeHuddle]:
		for mcKey, mcKeyFmt := range mcHuddleKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeMicrocosm]:
		for mcKey, mcKeyFmt := range mcMicrocosmKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypePoll]:
		for mcKey, mcKeyFmt := range mcPollKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeProfile]:
		for mcKey, mcKeyFmt := range mcProfileKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeQuestion]:
		for mcKey, mcKeyFmt := range mcQuestionKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeRole]:
		for mcKey, mcKeyFmt := range mcRoleKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	case h.ItemTypes[h.ItemTypeSite]:
		for mcKey, mcKeyFmt := range mcSiteKeys {
			if mcKey == scope {
				c.Delete(fmt.Sprintf(mcKeyFmt, itemID))
			}
		}

	default:
	}
}

// GetItemCacheKeys is used by Items.go as everything else knows what it is
// Only need to handle commentable types that exist as child items in microcosms
func GetItemCacheKeys(itemTypeID int64) map[int]string {
	switch itemTypeID {

	case h.ItemTypes[h.ItemTypeAlbum]:

	case h.ItemTypes[h.ItemTypeArticle]:

	case h.ItemTypes[h.ItemTypeClassified]:

	case h.ItemTypes[h.ItemTypeConversation]:
		return mcConversationKeys

	case h.ItemTypes[h.ItemTypeEvent]:
		return mcEventKeys

	case h.ItemTypes[h.ItemTypePoll]:
		return mcPollKeys

	case h.ItemTypes[h.ItemTypeQuestion]:
		return mcQuestionKeys

	default:
	}

	// Seriously should not reach here... things are likely to blow up
	return map[int]string{}
}

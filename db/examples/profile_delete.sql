-- get the deleted user id, in our case 47687
SELECT profile_id
  FROM profiles
 WHERE site_id = 234
   AND profile_name = 'deleted';

-- profile to delete = 93205

BEGIN;

UPDATE flags
   SET created_by = 47687
 WHERE created_by = 93205;
 
UPDATE search_index
   SET profile_id = 47687
 WHERE profile_id = 93205;

UPDATE revisions
   SET profile_id = 47687
 WHERE profile_id = 93205;

UPDATE comments
   SET profile_id = 47687
 WHERE profile_id = 93205;

UPDATE conversations
   SET created_by = 47687
 WHERE created_by = 93205;

UPDATE conversations
   SET edited_by = 47687
 WHERE edited_by = 93205;

UPDATE attendees
   SET created_by = 47687
 WHERE created_by = 93205;

UPDATE attendees
   SET attendee_id = 47687
 WHERE attendee_id = 93205;
 
UPDATE attendees
   SET profile_id = 47687
 WHERE profile_id = 93205;

UPDATE events
   SET created_by = 47687
 WHERE created_by = 93205;

DELETE FROM updates_latest
 WHERE update_id IN (SELECT update_id FROM updates
 WHERE for_profile_id = 93205);

DELETE FROM updates
 WHERE for_profile_id = 93205;

UPDATE updates
   SET created_by = 47687
 WHERE created_by = 93205;

DELETE FROM update_options
 WHERE profile_id = 93205;

DELETE FROM profile_options
 WHERE profile_id = 93205;

DELETE FROM read
 WHERE profile_id = 93205;

DELETE FROM role_profiles
 WHERE profile_id = 93205;

DELETE FROM huddle_profiles
 WHERE profile_id = 93205;

UPDATE huddles
   SET created_by = 47687
 WHERE created_by = 93205;

DELETE FROM watchers
 WHERE profile_id = 93205;

UPDATE profiles
   SET avatar_id = NULL
 WHERE profile_id = 93205
   AND avatar_id IS NOT NULL;

UPDATE attachments
   SET profile_id = 47687
 WHERE profile_id = 93205;

DELETE FROM follows
 WHERE profile_id = 93205;

UPDATE microcosms
   SET created_by = 47687
 WHERE created_by = 93205;

DELETE FROM ignores
 WHERE profile_id = 93205;

DELETE FROM ignores
 WHERE item_type_id = 3
   AND item_id = 93205;

DELETE FROM profiles
 WHERE profile_id = 93205;

ROLLBACK;
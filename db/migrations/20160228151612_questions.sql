
-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied

UPDATE item_types SET title = 'question' WHERE item_type_id = 10;

CREATE TABLE questions
(
  question_id bigserial NOT NULL,
  microcosm_id bigint NOT NULL,
  title character varying(150) NOT NULL,
  created timestamp without time zone NOT NULL,
  created_by bigint NOT NULL,
  edited timestamp without time zone,
  edited_by bigint,
  edit_reason character varying(150),
  is_sticky boolean NOT NULL DEFAULT false,
  is_open boolean NOT NULL DEFAULT true,
  is_deleted boolean NOT NULL DEFAULT false,
  is_moderated boolean NOT NULL DEFAULT false,
  is_visible boolean NOT NULL DEFAULT true,
  comment_count integer NOT NULL DEFAULT 0,
  view_count integer NOT NULL DEFAULT 0,
  accepted_answer_id bigint NOT NULL DEFAULT 0,
  CONSTRAINT questions_pkey PRIMARY KEY (question_id),
  CONSTRAINT questions_created_by_fkey FOREIGN KEY (created_by)
      REFERENCES profiles (profile_id) MATCH SIMPLE
      ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT questions_edited_by_fkey FOREIGN KEY (edited_by)
      REFERENCES profiles (profile_id) MATCH SIMPLE
      ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT questions_microcosm_id_fkey FOREIGN KEY (microcosm_id)
      REFERENCES microcosms (microcosm_id) MATCH SIMPLE
      ON UPDATE NO ACTION ON DELETE NO ACTION
)
WITH (
  OIDS=FALSE
);
ALTER TABLE questions
  OWNER TO microcosm;
COMMENT ON TABLE questions
  IS 'Questions are a vote ordered collection of comments.

One comment *may* be accepted by a moderator or the question askee as the
accepted answer to the question.

This is Stack Overflow in a forum.';

CREATE INDEX questions_isdeleted_idx ON questions USING btree (is_deleted);
CREATE INDEX questions_microcosmid_idx ON questions USING btree (microcosm_id);
CREATE INDEX questions_acceptedanswerid_idx ON questions USING btree (accepted_answer_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_questions_flags()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') THEN

            DELETE
              FROM flags
             WHERE item_type_id = 10
               AND item_id = OLD.question_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.is_deleted <> OLD.is_deleted OR
               NEW.is_moderated <> OLD.is_moderated OR
               NEW.is_sticky <> OLD.is_sticky OR
               NEW.microcosm_id <> OLD.microcosm_id THEN

            -- Item
            UPDATE flags AS f
               SET microcosm_is_deleted = m.is_deleted
                  ,microcosm_is_moderated = m.is_moderated
                  ,item_is_deleted = NEW.is_deleted
                  ,item_is_moderated = NEW.is_moderated
                  ,item_is_sticky = NEW.is_sticky
                  ,microcosm_id = NEW.microcosm_id
              FROM microcosms m
             WHERE m.microcosm_id = NEW.microcosm_id
               AND f.item_type_id = 10
               AND f.item_id = NEW.question_id;

            -- Children (comments)
            UPDATE flags
               SET microcosm_is_deleted = m.is_deleted
                  ,microcosm_is_moderated = m.is_moderated
                  ,parent_is_deleted = NEW.is_deleted
                  ,parent_is_moderated = NEW.is_moderated
                  ,microcosm_id = NEW.microcosm_id
              FROM microcosms m
             WHERE m.microcosm_id = NEW.microcosm_id
               AND parent_item_type_id = 10
               AND parent_item_id = NEW.question_id;

            END IF;

            RETURN NEW;

        ELSIF (TG_OP = 'INSERT') THEN

            INSERT INTO flags (
                site_id
               ,microcosm_id
               ,microcosm_is_deleted
               ,microcosm_is_moderated
               ,item_type_id
               ,item_id
               ,item_is_deleted
               ,item_is_moderated
               ,item_is_sticky
               ,last_modified
               ,created_by
            )
            SELECT m.site_id
                  ,NEW.microcosm_id
                  ,m.is_deleted
                  ,m.is_moderated
                  ,10
                  ,NEW.question_id
                  ,NEW.is_deleted
                  ,NEW.is_moderated
                  ,NEW.is_sticky
                  ,NEW.created
                  ,NEW.created_by
              FROM microcosms AS m
             WHERE m.microcosm_id = NEW.microcosm_id;

            UPDATE flags
               SET last_modified = NEW.created
             WHERE item_type_id = 2
               AND item_id = NEW.microcosm_id;

            RETURN NEW;
        END IF;
        RETURN NULL; -- result is ignored since this is an AFTER trigger
    END;
$BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION update_questions_flags()
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_questions_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 10
               AND item_id = OLD.question_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.title <> OLD.title THEN
        
                UPDATE search_index
                   SET title_text = NEW.title
                      ,title_vector = setweight(to_tsvector(NEW.title), 'C')
                      ,document_text = NEW.title
                      ,document_vector = setweight(to_tsvector(NEW.title), 'C')
                      ,last_modified = NOW()
                 WHERE item_type_id = 10
                   AND item_id = NEW.question_id;

            END IF;

            IF NEW.microcosm_id <> OLD.microcosm_id THEN

                UPDATE search_index
                   SET microcosm_id = NEW.microcosm_id
                 WHERE (
                           (item_type_id = 10 AND item_id = NEW.question_id)
                        OR (parent_item_type_id = 10 AND parent_item_id = NEW.question_id)
                       )
                   AND microcosm_id <> NEW.microcosm_id;

            END IF;

            RETURN NEW;

        ELSIF (TG_OP = 'INSERT') THEN
            INSERT INTO search_index (
                site_id
               ,microcosm_id
               ,profile_id
               ,item_type_id
               ,item_id

               ,title_text
               ,title_vector
               ,document_text
               ,document_vector
               ,last_modified
            )
            SELECT m.site_id
                  ,NEW.microcosm_id
                  ,NEW.created_by
                  ,10
                  ,NEW.question_id
                  ,NEW.title

                  ,setweight(to_tsvector(NEW.title), 'C')
                  ,NEW.title
                  ,setweight(to_tsvector(NEW.title), 'C')
                  ,NOW()
              FROM microcosms m
             WHERE m.microcosm_id = NEW.microcosm_id;

            RETURN NEW;
        END IF;
        RETURN NULL; -- result is ignored since this is an AFTER trigger
    END;
$BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION update_questions_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

CREATE TRIGGER questions_flags
  AFTER INSERT OR UPDATE OR DELETE
  ON questions
  FOR EACH ROW
  EXECUTE PROCEDURE update_questions_flags();

CREATE TRIGGER questions_search_index
  AFTER INSERT OR UPDATE OR DELETE
  ON questions
  FOR EACH ROW
  EXECUTE PROCEDURE update_questions_search_index();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION has_unread(
    in_item_type_id bigint DEFAULT 0,
    in_item_id bigint DEFAULT 0,
    in_profile_id bigint DEFAULT 0)
  RETURNS boolean AS
$BODY$
DECLARE
BEGIN

    IF in_profile_id = 0 THEN
        RETURN false;
    END IF;

    CASE in_item_type_id
    WHEN 1 THEN -- site
    WHEN 2 THEN -- microcosm

        -- Check child forums should they exist
        IF (SELECT COALESCE(BOOL_OR(has_unread(2, m.microcosm_id, in_profile_id)), FALSE)
              FROM microcosms m
              LEFT JOIN permissions_cache p ON p.site_id = m.site_id
                                           AND p.item_type_id = 2
                                           AND p.item_id = m.microcosm_id
                                           AND p.profile_id = in_profile_id
                   LEFT JOIN ignores_expanded i ON i.profile_id = in_profile_id
                                               AND i.item_type_id = 2
                                               AND i.item_id = m.microcosm_id
             WHERE m.parent_id = in_item_id
               AND m.is_deleted IS NOT TRUE
               AND m.is_moderated IS NOT TRUE
               AND i.profile_id IS NULL
               AND (
                       (p.can_read IS NOT NULL AND p.can_read IS TRUE)
                    OR (get_effective_permissions(m.site_id,m.microcosm_id,2,m.microcosm_id,in_profile_id)).can_read IS TRUE
                   )) THEN
            RETURN TRUE;
        END IF;

        -- Check the last read of the microcosm against the read time
        IF NOT (SELECT COALESCE(
                (
                    SELECT last_modified
                      FROM flags
                     WHERE microcosm_id = in_item_id
                       AND item_type_id IN (6, 9, 10)
                       AND NOT item_is_deleted
                       AND NOT item_is_moderated
                       AND last_modified > (
                               SELECT read
                                 FROM read
                                WHERE profile_id = in_profile_id
                                  AND item_type_id = in_item_type_id
                                  AND item_id = in_item_id
                           )
                     ORDER BY last_modified DESC
                     LIMIT 1
                ) > (
                    SELECT read
                      FROM read
                     WHERE profile_id = in_profile_id
                       AND item_type_id = in_item_type_id
                       AND item_id = in_item_id
                ), true)) THEN
            RETURN false;
        END IF;

        -- We don't have a recent last_read indicator, and need to call
        -- has_unread for items... but if we do have an old read row for
        -- the microcosm then we only need to check the items since that
        -- time.
        IF (SELECT COUNT(*)
              FROM read
             WHERE profile_id = in_profile_id
               AND item_type_id = in_item_type_id
               AND item_id = in_item_id ) > 0 THEN

            RETURN (SELECT EXISTS(
                        SELECT 1
                          FROM flags
                         WHERE microcosm_id = in_item_id
                           AND item_type_id IN (6, 9, 10)
                           AND NOT item_is_deleted
                           AND NOT item_is_moderated
                           AND last_modified > (
                                   SELECT read
                                     FROM read
                                    WHERE profile_id = in_profile_id
                                      AND item_type_id = in_item_type_id
                                      AND item_id = in_item_id
                               )
                           AND has_unread(item_type_id, item_id, in_profile_id)
                         ORDER BY last_modified DESC
                   ));

        ELSE

            -- The really slow way, iterate every item until we hit something unread
            RETURN (SELECT EXISTS(
                        SELECT 1
                          FROM flags
                         WHERE microcosm_id = in_item_id
                           AND item_type_id IN (6, 9, 10)
                           AND NOT item_is_deleted
                           AND NOT item_is_moderated
                           AND has_unread(item_type_id, item_id, in_profile_id)
                         ORDER BY last_modified DESC
                   ));
        END IF;

    WHEN 3 THEN -- profile
    WHEN 4 THEN -- comment
    WHEN 5 THEN -- huddle

        -- Check the last read of all huddles against the read time
        IF NOT (SELECT COALESCE(
                (
                    SELECT last_modified
                      FROM flags
                     WHERE item_type_id = 5
                       AND item_id = in_item_id
                       AND NOT item_is_deleted
                       AND NOT item_is_moderated
                       AND last_modified > (
                               SELECT read
                                 FROM read
                                WHERE profile_id = in_profile_id
                                  AND item_type_id = 5
                                  AND item_id = 0
                           )
                ) > (
                    SELECT read
                      FROM read
                     WHERE profile_id = in_profile_id
                       AND item_type_id = 5
                       AND item_id = 0
                ), true)) THEN
            RETURN false;
        END IF;


        -- We don't have a recent last_read indicator, and need to call
        -- has_unread for items... but if we do have an old read row for
        -- all huddles then we only need to check the items since that
        -- time.
        IF (SELECT EXISTS(
            SELECT 1
              FROM read
             WHERE profile_id = in_profile_id
               AND item_type_id = 5
               AND item_id = 0 )) THEN

            RETURN (SELECT EXISTS(
                       SELECT 1
                         FROM (            
                        SELECT COALESCE(f.last_modified > GREATEST(MAX(r.read), r2.read), true) AS unread
                          FROM flags f
                               LEFT JOIN read r ON r.item_type_id = 5
                                               AND r.item_id = in_item_id
                                               AND r.profile_id = in_profile_id
                              ,(
                                   SELECT read
                                     FROM read
                                    WHERE profile_id = in_profile_Id
                                      AND item_type_id = 5
                                      AND item_id = 0
                               ) r2
                         WHERE f.item_type_id = 5
                           AND f.item_id = in_item_id
                           AND f.item_is_deleted IS NOT TRUE
                           AND f.item_is_moderated IS NOT TRUE
                         GROUP BY f.last_modified, r2.read
                               ) as u
                         WHERE unread
                   ));

        ELSE

            -- The really slow way, iterate every item until we hit something unread
            RETURN (SELECT EXISTS(
                        SELECT 1
                          FROM (
                                SELECT COALESCE(i.last_modified > MAX(r.read), true) AS unread
                                  FROM flags i
                                       LEFT JOIN read r ON r.item_type_id = in_item_type_id
                                                       AND r.item_id = in_item_id
                                                       AND r.profile_id = in_profile_id
                                 WHERE i.item_type_id = in_item_type_id
                                   AND i.item_id = in_item_id
                                 GROUP BY r.read, i.last_modified
                               ) AS u
                         WHERE unread
                   ));

        END IF;

    WHEN 6 THEN -- conversation

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 7 THEN -- poll

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 8 THEN -- article

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 9 THEN -- event

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 10 THEN -- question

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 11 THEN -- classified

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 12 THEN -- album

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 13 THEN -- attendee
    WHEN 14 THEN -- user
    WHEN 15 THEN -- attribute
    WHEN 16 THEN -- update
    WHEN 17 THEN -- role
    WHEN 18 THEN -- update type
    WHEN 19 THEN -- watcher
    END CASE;

    RETURN false;

END;
$BODY$
  LANGUAGE plpgsql STABLE
  COST 100;
ALTER FUNCTION has_unread(bigint, bigint, bigint)
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back

DROP TRIGGER questions_search_index ON questions;
DROP TRIGGER questions_flags ON questions;
DROP FUNCTION update_questions_search_index();
DROP FUNCTION update_questions_flags();
DROP TABLE questions;

UPDATE item_types SET title = 'q_and_a' WHERE item_type_id = 10;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION has_unread(
    in_item_type_id bigint DEFAULT 0,
    in_item_id bigint DEFAULT 0,
    in_profile_id bigint DEFAULT 0)
  RETURNS boolean AS
$BODY$
DECLARE
BEGIN

    IF in_profile_id = 0 THEN
        RETURN false;
    END IF;

    CASE in_item_type_id
    WHEN 1 THEN -- site
    WHEN 2 THEN -- microcosm

        -- Check child forums should they exist
        IF (SELECT COALESCE(BOOL_OR(has_unread(2, m.microcosm_id, in_profile_id)), FALSE)
              FROM microcosms m
              LEFT JOIN permissions_cache p ON p.site_id = m.site_id
                                           AND p.item_type_id = 2
                                           AND p.item_id = m.microcosm_id
                                           AND p.profile_id = in_profile_id
                   LEFT JOIN ignores_expanded i ON i.profile_id = in_profile_id
                                               AND i.item_type_id = 2
                                               AND i.item_id = m.microcosm_id
             WHERE m.parent_id = in_item_id
               AND m.is_deleted IS NOT TRUE
               AND m.is_moderated IS NOT TRUE
               AND i.profile_id IS NULL
               AND (
                       (p.can_read IS NOT NULL AND p.can_read IS TRUE)
                    OR (get_effective_permissions(m.site_id,m.microcosm_id,2,m.microcosm_id,in_profile_id)).can_read IS TRUE
                   )) THEN
            RETURN TRUE;
        END IF;

        -- Check the last read of the microcosm against the read time
        IF NOT (SELECT COALESCE(
                (
                    SELECT last_modified
                      FROM flags
                     WHERE microcosm_id = in_item_id
                       AND item_type_id IN (6, 9)
                       AND NOT item_is_deleted
                       AND NOT item_is_moderated
                       AND last_modified > (
                               SELECT read
                                 FROM read
                                WHERE profile_id = in_profile_id
                                  AND item_type_id = in_item_type_id
                                  AND item_id = in_item_id
                           )
                     ORDER BY last_modified DESC
                     LIMIT 1
                ) > (
                    SELECT read
                      FROM read
                     WHERE profile_id = in_profile_id
                       AND item_type_id = in_item_type_id
                       AND item_id = in_item_id
                ), true)) THEN
            RETURN false;
        END IF;

        -- We don't have a recent last_read indicator, and need to call
        -- has_unread for items... but if we do have an old read row for
        -- the microcosm then we only need to check the items since that
        -- time.
        IF (SELECT COUNT(*)
              FROM read
             WHERE profile_id = in_profile_id
               AND item_type_id = in_item_type_id
               AND item_id = in_item_id ) > 0 THEN

            RETURN (SELECT EXISTS(
                        SELECT 1
                          FROM flags
                         WHERE microcosm_id = in_item_id
                           AND item_type_id IN (6, 9)
                           AND NOT item_is_deleted
                           AND NOT item_is_moderated
                           AND last_modified > (
                                   SELECT read
                                     FROM read
                                    WHERE profile_id = in_profile_id
                                      AND item_type_id = in_item_type_id
                                      AND item_id = in_item_id
                               )
                           AND has_unread(item_type_id, item_id, in_profile_id)
                         ORDER BY last_modified DESC
                   ));

        ELSE

            -- The really slow way, iterate every item until we hit something unread
            RETURN (SELECT EXISTS(
                        SELECT 1
                          FROM flags
                         WHERE microcosm_id = in_item_id
                           AND item_type_id IN (6, 9)
                           AND NOT item_is_deleted
                           AND NOT item_is_moderated
                           AND has_unread(item_type_id, item_id, in_profile_id)
                         ORDER BY last_modified DESC
                   ));
        END IF;

    WHEN 3 THEN -- profile
    WHEN 4 THEN -- comment
    WHEN 5 THEN -- huddle

        -- Check the last read of all huddles against the read time
        IF NOT (SELECT COALESCE(
                (
                    SELECT last_modified
                      FROM flags
                     WHERE item_type_id = 5
                       AND item_id = in_item_id
                       AND NOT item_is_deleted
                       AND NOT item_is_moderated
                       AND last_modified > (
                               SELECT read
                                 FROM read
                                WHERE profile_id = in_profile_id
                                  AND item_type_id = 5
                                  AND item_id = 0
                           )
                ) > (
                    SELECT read
                      FROM read
                     WHERE profile_id = in_profile_id
                       AND item_type_id = 5
                       AND item_id = 0
                ), true)) THEN
            RETURN false;
        END IF;


        -- We don't have a recent last_read indicator, and need to call
        -- has_unread for items... but if we do have an old read row for
        -- all huddles then we only need to check the items since that
        -- time.
        IF (SELECT EXISTS(
            SELECT 1
              FROM read
             WHERE profile_id = in_profile_id
               AND item_type_id = 5
               AND item_id = 0 )) THEN

            RETURN (SELECT EXISTS(
                       SELECT 1
                         FROM (            
                        SELECT COALESCE(f.last_modified > GREATEST(MAX(r.read), r2.read), true) AS unread
                          FROM flags f
                               LEFT JOIN read r ON r.item_type_id = 5
                                               AND r.item_id = in_item_id
                                               AND r.profile_id = in_profile_id
                              ,(
                                   SELECT read
                                     FROM read
                                    WHERE profile_id = in_profile_Id
                                      AND item_type_id = 5
                                      AND item_id = 0
                               ) r2
                         WHERE f.item_type_id = 5
                           AND f.item_id = in_item_id
                           AND f.item_is_deleted IS NOT TRUE
                           AND f.item_is_moderated IS NOT TRUE
                         GROUP BY f.last_modified, r2.read
                               ) as u
                         WHERE unread
                   ));

        ELSE

            -- The really slow way, iterate every item until we hit something unread
            RETURN (SELECT EXISTS(
                        SELECT 1
                          FROM (
                                SELECT COALESCE(i.last_modified > MAX(r.read), true) AS unread
                                  FROM flags i
                                       LEFT JOIN read r ON r.item_type_id = in_item_type_id
                                                       AND r.item_id = in_item_id
                                                       AND r.profile_id = in_profile_id
                                 WHERE i.item_type_id = in_item_type_id
                                   AND i.item_id = in_item_id
                                 GROUP BY r.read, i.last_modified
                               ) AS u
                         WHERE unread
                   ));

        END IF;

    WHEN 6 THEN -- conversation

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 7 THEN -- poll

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 8 THEN -- article

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 9 THEN -- event

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 10 THEN -- question

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 11 THEN -- classified

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 12 THEN -- album

        RETURN COUNT(*) > 0 AS has_unread
          FROM (
                SELECT COALESCE(i.last_modified > r.read, true) AS unread
                  FROM flags i
                       LEFT JOIN "read" r
                         ON (
                                (r.item_type_id = i.item_type_id AND r.item_id = i.item_id) 
                             OR (r.item_type_id = 2 AND r.item_id = i.microcosm_id)
                            )
                        AND r.profile_id = in_profile_id
                 WHERE i.item_type_id = in_item_type_id
                   AND i.item_id = in_item_id
                 ORDER BY r.read DESC
                 LIMIT 1
               ) AS u
         WHERE unread;

    WHEN 13 THEN -- attendee
    WHEN 14 THEN -- user
    WHEN 15 THEN -- attribute
    WHEN 16 THEN -- update
    WHEN 17 THEN -- role
    WHEN 18 THEN -- update type
    WHEN 19 THEN -- watcher
    END CASE;

    RETURN false;

END;
$BODY$
  LANGUAGE plpgsql STABLE
  COST 100;
ALTER FUNCTION has_unread(bigint, bigint, bigint)
  OWNER TO microcosm;
-- +goose StatementEnd
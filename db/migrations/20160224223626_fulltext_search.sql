
-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied

CREATE INDEX search_index_microcosm_id_idx ON search_index USING btree (microcosm_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_comment_microcosm_id(
    incommentid bigint DEFAULT 0)
  RETURNS BIGINT AS
$BODY$
DECLARE
BEGIN

    RETURN CASE WHEN c.item_type_id = 6 THEN
               (SELECT microcosm_id FROM conversations WHERE conversation_id = c.item_id)
                WHEN c.item_type_id = 9 THEN
               (SELECT microcosm_id FROM events WHERE event_id = c.item_id)
                ELSE NULL
            END AS microcosm_id
           FROM "comments" c
          WHERE c.comment_id = incommentid;

END;
$BODY$
  LANGUAGE plpgsql STABLE
  COST 100;
ALTER FUNCTION get_comment_microcosm_id(bigint)
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_revisions_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') AND OLD.is_current THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 4
               AND item_id = OLD.comment_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.raw <> OLD.raw THEN

            UPDATE search_index
               SET document_text = NEW.raw
                  ,document_vector = setweight(to_tsvector(NEW.raw), 'D')
                  ,last_modified = NOW()
             WHERE item_type_id = 4
               AND item_id = NEW.comment_id;

            END IF;

            RETURN NEW;

        ELSIF (TG_OP = 'INSERT') THEN

            IF (
                SELECT COUNT(*) = 0 
                  FROM search_index
                 WHERE item_type_id = 4
                   AND item_id = NEW.comment_id
            ) THEN

                INSERT INTO search_index (
                    site_id
                   ,profile_id
                   ,item_type_id
                   ,item_id
                   ,parent_item_type_id

                   ,parent_item_id
                   ,document_text
                   ,document_vector
                   ,last_modified
                   ,microcosm_id
                )
                SELECT p.site_id
                      ,p.profile_id
                      ,4
                      ,NEW.comment_id
                      ,c.item_type_id

                      ,c.item_id
                      ,NEW.raw
                      ,setweight(to_tsvector(NEW.raw), 'D')
                      ,NEW.created
                      ,get_comment_microcosm_id(NEW.comment_id)
                  FROM profiles p
                      ,"comments" c
                 WHERE p.profile_id = NEW.profile_id
                   AND c.comment_id = NEW.comment_id;

            ELSE

                UPDATE search_index
                   SET document_text = NEW.raw
                      ,document_vector = setweight(to_tsvector(NEW.raw), 'D')
                      ,last_modified = NOW()
                 WHERE item_type_id = 4
                   AND item_id = NEW.comment_id;

            END IF;

            RETURN NEW;
        END IF;
        RETURN NULL; -- result is ignored since this is an AFTER trigger
    END;
$BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION update_revisions_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_conversations_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 6
               AND item_id = OLD.conversation_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.microcosm_id <> OLD.microcosm_id OR
               NEW.title <> OLD.title THEN
        
            UPDATE search_index
               SET microcosm_id = NEW.microcosm_id
                  ,title_text = NEW.title
                  ,title_vector = setweight(to_tsvector(NEW.title), 'C')
                  ,document_text = NEW.title
                  ,document_vector = setweight(to_tsvector(NEW.title), 'C')
                  ,last_modified = NOW()
             WHERE item_type_id = 6
               AND item_id = NEW.conversation_id;

            UPDATE search_index
               SET microcosm_id = NEW.microcosm_id
             WHERE parent_item_type_id = 6
               AND parent_item_id = NEW.conversation_id
               AND microcosm_id != NEW.microcosm_id;

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
                  ,6
                  ,NEW.conversation_id
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
ALTER FUNCTION update_conversations_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_events_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 9
               AND item_id = OLD.event_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.microcosm_id <> OLD.microcosm_id OR
               NEW.title <> OLD.title THEN

            UPDATE search_index
               SET microcosm_id = NEW.microcosm_id
                  ,title_text = NEW.title
                  ,title_vector = setweight(to_tsvector(NEW.title), 'C')
                  ,document_text = NEW.title || ' ' || COALESCE(NEW.where, '')
                  ,document_vector = setweight(to_tsvector(NEW.title), 'C') || setweight(to_tsvector(COALESCE(NEW.where, '')), 'D')
                  ,last_modified = NOW()
             WHERE item_type_id = 9
               AND item_id = NEW.event_id;

            UPDATE search_index
               SET microcosm_id = NEW.microcosm_id
             WHERE parent_item_type_id = 9
               AND parent_item_id = NEW.event_id
               AND microcosm_id != NEW.microcosm_id;

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
                  ,m.microcosm_id
                  ,NEW.created_by
                  ,9
                  ,NEW.event_id

                  ,NEW.title
                  ,setweight(to_tsvector(NEW.title), 'C')
                  ,NEW.title || ' ' || COALESCE(NEW.where, '')
                  ,setweight(to_tsvector(NEW.title), 'C') || setweight(to_tsvector(COALESCE(NEW.where, '')), 'D')
                  ,NEW.created
              FROM microcosms m
             WHERE m.microcosm_id = NEW.microcosm_id;

            RETURN NEW;
        END IF;
        RETURN NULL; -- result is ignored since this is an AFTER trigger
    END;
$BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION update_events_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

UPDATE search_index si
   SET microcosm_id = f.microcosm_id
  FROM flags AS f
 WHERE si.item_type_id = 4
   AND (si.microcosm_id IS NULL OR si.microcosm_id != f.microcosm_id)
   AND si.parent_item_type_id != 5
   AND si.parent_item_type_id != 3
   AND f.item_type_id = si.parent_item_type_id
   AND f.item_id = si.parent_item_id;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_conversations_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 6
               AND item_id = OLD.conversation_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.microcosm_id <> OLD.microcosm_id OR
               NEW.title <> OLD.title THEN
        
            UPDATE search_index
               SET microcosm_id = NEW.microcosm_id
                  ,title_text = NEW.title
                  ,title_vector = setweight(to_tsvector(NEW.title), 'C')
                  ,document_text = NEW.title
                  ,document_vector = setweight(to_tsvector(NEW.title), 'C')
                  ,last_modified = NOW()
             WHERE item_type_id = 6
               AND item_id = NEW.conversation_id;

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
                  ,6
                  ,NEW.conversation_id
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
ALTER FUNCTION update_conversations_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_events_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 9
               AND item_id = OLD.event_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.microcosm_id <> OLD.microcosm_id OR
               NEW.title <> OLD.title THEN

            UPDATE search_index
               SET microcosm_id = NEW.microcosm_id
                  ,title_text = NEW.title
                  ,title_vector = setweight(to_tsvector(NEW.title), 'C')
                  ,document_text = NEW.title || ' ' || COALESCE(NEW.where, '')
                  ,document_vector = setweight(to_tsvector(NEW.title), 'C') || setweight(to_tsvector(COALESCE(NEW.where, '')), 'D')
                  ,last_modified = NOW()
             WHERE item_type_id = 9
               AND item_id = NEW.event_id;

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
                  ,m.microcosm_id
                  ,NEW.created_by
                  ,9
                  ,NEW.event_id

                  ,NEW.title
                  ,setweight(to_tsvector(NEW.title), 'C')
                  ,NEW.title || ' ' || COALESCE(NEW.where, '')
                  ,setweight(to_tsvector(NEW.title), 'C') || setweight(to_tsvector(COALESCE(NEW.where, '')), 'D')
                  ,NEW.created
              FROM microcosms m
             WHERE m.microcosm_id = NEW.microcosm_id;

            RETURN NEW;
        END IF;
        RETURN NULL; -- result is ignored since this is an AFTER trigger
    END;
$BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION update_events_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_revisions_search_index()
  RETURNS trigger AS
$BODY$
    BEGIN
        IF (TG_OP = 'DELETE') AND OLD.is_current THEN
            DELETE
              FROM search_index
             WHERE item_type_id = 4
               AND item_id = OLD.comment_id;

            RETURN OLD;

        ELSIF (TG_OP = 'UPDATE') THEN

            IF NEW.raw <> OLD.raw THEN

            UPDATE search_index
               SET document_text = NEW.raw
                  ,document_vector = setweight(to_tsvector(NEW.raw), 'D')
                  ,last_modified = NOW()
             WHERE item_type_id = 4
               AND item_id = NEW.comment_id;

            END IF;

            RETURN NEW;

        ELSIF (TG_OP = 'INSERT') THEN

            IF (
                SELECT COUNT(*) = 0 
                  FROM search_index
                 WHERE item_type_id = 4
                   AND item_id = NEW.comment_id
            ) THEN

                INSERT INTO search_index (
                    site_id
                   ,profile_id
                   ,item_type_id
                   ,item_id
                   ,parent_item_type_id

                   ,parent_item_id
                   ,document_text
                   ,document_vector
                   ,last_modified
                )
                SELECT p.site_id
                      ,p.profile_id
                      ,4
                      ,NEW.comment_id
                      ,c.item_type_id

                      ,c.item_id
                      ,NEW.raw
                      ,setweight(to_tsvector(NEW.raw), 'D')
                      ,NEW.created
                  FROM profiles p
                      ,"comments" c
                 WHERE p.profile_id = NEW.profile_id
                   AND c.comment_id = NEW.comment_id;

            ELSE

                UPDATE search_index
                   SET document_text = NEW.raw
                      ,document_vector = setweight(to_tsvector(NEW.raw), 'D')
                      ,last_modified = NOW()
                 WHERE item_type_id = 4
                   AND item_id = NEW.comment_id;

            END IF;

            RETURN NEW;
        END IF;
        RETURN NULL; -- result is ignored since this is an AFTER trigger
    END;
$BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION update_revisions_search_index()
  OWNER TO microcosm;
-- +goose StatementEnd

DROP FUNCTION get_comment_microcosm_id(bigint);
DROP INDEX search_index_microcosm_id_idx;

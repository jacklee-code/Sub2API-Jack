-- Jack chat mode: unread replies, per-conversation web search, and a stable
-- context start so long conversations stay within the model window.
ALTER TABLE jack_chat_conversations
 ADD COLUMN last_reply_at timestamptz,
 ADD COLUMN read_at timestamptz,
 ADD COLUMN web_search boolean NOT NULL DEFAULT true,
 ADD COLUMN context_start_id bigint;

-- Existing replies were already seen.
UPDATE jack_chat_conversations SET last_reply_at = updated_at, read_at = updated_at;

CREATE INDEX jack_chat_conversations_unread ON jack_chat_conversations(user_id) WHERE last_reply_at IS NOT NULL;

ALTER TABLE jack_chat_settings ADD COLUMN max_context_tokens integer NOT NULL DEFAULT 200000;

-- Ratios outside the supported sizes fall back to square.
UPDATE jack_chat_conversations SET image_aspect = '1:1' WHERE image_aspect NOT IN ('1:1', '3:2', '2:3', 'auto');
UPDATE jack_chat_preferences SET image_aspect = '1:1' WHERE image_aspect NOT IN ('1:1', '3:2', '2:3', 'auto');

-- Jack chat mode: web conversations that call the gateway through hidden
-- per-(user, group) API keys (prefix sk-jackchat-). Billing rows stay upstream.
CREATE TABLE jack_chat_settings (
 id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
 enabled boolean NOT NULL DEFAULT true,
 system_prompt text NOT NULL DEFAULT '',
 max_image_bytes bigint NOT NULL DEFAULT 20971520,
 max_pdf_bytes bigint NOT NULL DEFAULT 33554432,
 max_text_bytes bigint NOT NULL DEFAULT 2097152,
 max_attachments integer NOT NULL DEFAULT 10,
 max_conversations integer NOT NULL DEFAULT 500,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO jack_chat_settings(id) VALUES (1);

CREATE TABLE jack_chat_preferences (
 user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 mode varchar(10) NOT NULL DEFAULT 'chat',
 group_id bigint,
 chat_model varchar(200) NOT NULL DEFAULT '',
 reasoning_effort varchar(20) NOT NULL DEFAULT '',
 image_model varchar(200) NOT NULL DEFAULT '',
 image_aspect varchar(10) NOT NULL DEFAULT '1:1',
 image_count smallint NOT NULL DEFAULT 1,
 web_search boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE jack_chat_conversations (
 id bigserial PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 mode varchar(10) NOT NULL DEFAULT 'chat' CHECK (mode IN ('chat', 'image')),
 group_id bigint,
 model varchar(200) NOT NULL DEFAULT '',
 reasoning_effort varchar(20) NOT NULL DEFAULT '',
 image_aspect varchar(10) NOT NULL DEFAULT '1:1',
 image_count smallint NOT NULL DEFAULT 1,
 title varchar(200) NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jack_chat_conversations_user ON jack_chat_conversations(user_id, updated_at DESC);

CREATE TABLE jack_chat_messages (
 id bigserial PRIMARY KEY,
 conversation_id bigint NOT NULL REFERENCES jack_chat_conversations(id) ON DELETE CASCADE,
 role varchar(16) NOT NULL CHECK (role IN ('user', 'assistant')),
 content text NOT NULL DEFAULT '',
 reasoning text NOT NULL DEFAULT '',
 model varchar(200) NOT NULL DEFAULT '',
 reasoning_effort varchar(20) NOT NULL DEFAULT '',
 status varchar(16) NOT NULL DEFAULT 'complete',
 error text NOT NULL DEFAULT '',
 input_tokens integer NOT NULL DEFAULT 0,
 output_tokens integer NOT NULL DEFAULT 0,
 web_search boolean NOT NULL DEFAULT false,
 citations jsonb NOT NULL DEFAULT '[]'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jack_chat_messages_conversation ON jack_chat_messages(conversation_id, id);

CREATE TABLE jack_chat_attachments (
 id bigserial PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 message_id bigint REFERENCES jack_chat_messages(id) ON DELETE CASCADE,
 kind varchar(16) NOT NULL CHECK (kind IN ('image', 'pdf', 'text', 'generated')),
 storage_key text NOT NULL DEFAULT '',
 filename varchar(255) NOT NULL DEFAULT '',
 mime varchar(100) NOT NULL DEFAULT '',
 size bigint NOT NULL DEFAULT 0,
 width integer NOT NULL DEFAULT 0,
 height integer NOT NULL DEFAULT 0,
 extracted_text text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jack_chat_attachments_message ON jack_chat_attachments(message_id);
CREATE INDEX jack_chat_attachments_pending ON jack_chat_attachments(user_id, created_at) WHERE message_id IS NULL;
CREATE INDEX jack_chat_attachments_storage ON jack_chat_attachments(storage_key) WHERE storage_key <> '';

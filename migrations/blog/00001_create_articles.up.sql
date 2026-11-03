CREATE SCHEMA IF NOT EXISTS blog;

CREATE TABLE blog.articles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id BIGINT NOT NULL CHECK (author_id > 0),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT articles_title_length
        CHECK (char_length(title) BETWEEN 1 AND 200),

    CONSTRAINT articles_body_length
        CHECK (char_length(body) BETWEEN 1 AND 50000)
);

CREATE INDEX articles_created_at_id_idx
    ON blog.articles (created_at DESC, id DESC);

-- +goose Up
CREATE TABLE widgets (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT widgets_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT widgets_name_length CHECK (char_length(name) <= 120)
);

CREATE INDEX widgets_created_at_id_idx ON widgets (created_at DESC, id DESC);

-- +goose Down
DROP TABLE widgets;

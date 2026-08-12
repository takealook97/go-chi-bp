-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION set_widgets_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER widgets_set_updated_at
BEFORE UPDATE ON widgets
FOR EACH ROW
EXECUTE FUNCTION set_widgets_updated_at();

-- +goose Down
DROP TRIGGER widgets_set_updated_at ON widgets;
DROP FUNCTION set_widgets_updated_at();

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION create_monthly_partition(target_date DATE DEFAULT CURRENT_DATE)
RETURNS TEXT AS $$
DECLARE
    start_date DATE := DATE_TRUNC('month', target_date)::DATE;
    end_date   DATE := (start_date + INTERVAL '1 month')::DATE;
    part_name  TEXT := 'messages_' || TO_CHAR(start_date, 'YYYY_MM');
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = part_name) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF messages FOR VALUES FROM (%L) TO (%L)',
            part_name, start_date, end_date
        );
        RETURN 'created: ' || part_name;
    END IF;
    RETURN 'exists: ' || part_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS create_monthly_partition;
-- +goose StatementEnd
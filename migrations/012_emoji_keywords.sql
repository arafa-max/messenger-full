-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS emoji_keywords (
    id          SERIAL PRIMARY KEY,
    keyword     VARCHAR(64) NOT NULL,
    emoji       VARCHAR(10) NOT NULL,
    lang        VARCHAR(5)  DEFAULT 'ru',
    weight      INT         DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emoji_keywords_uniq ON emoji_keywords(keyword, emoji);
CREATE INDEX IF NOT EXISTS idx_emoji_keywords_kw ON emoji_keywords USING gin(keyword gin_trgm_ops);

INSERT INTO emoji_keywords (keyword, emoji, lang, weight) VALUES
('смеюсь','😂','ru',10),('смешно','😂','ru',10),('хаха','😂','ru',10),
('ржу','😂','ru',9),('лол','😂','ru',9),
('привет','👋','ru',10),('хай','👋','ru',10),('здарова','👋','ru',9),
('пока','👋','ru',10),('бай','👋','ru',9),
('люблю','❤️','ru',10),('любовь','❤️','ru',10),('обожаю','❤️','ru',9),
('злой','😡','ru',10),('бесит','😡','ru',10),('злюсь','😡','ru',9),
('грустно','😢','ru',10),('плачу','😢','ru',10),('печаль','😢','ru',9),
('огонь','🔥','ru',10),('круто','🔥','ru',10),('топ','🔥','ru',9),
('окей','👍','ru',10),('ок','👍','ru',10),('хорошо','👍','ru',9),('согласен','👍','ru',9),
('нет','👎','ru',10),('плохо','👎','ru',8),
('спасибо','🙏','ru',10),('спс','🙏','ru',9),
('вау','😮','ru',10),('ого','😮','ru',10),('офигеть','😮','ru',9),
('думаю','🤔','ru',10),('хмм','🤔','ru',10),('интересно','🤔','ru',9),
('устал','😴','ru',10),('сплю','😴','ru',10),
('голодный','🍕','ru',9),
('ура','🎉','ru',10),('праздник','🎉','ru',10),('поздравляю','🎉','ru',9),
('день рождения','🎂','ru',10),
('lol','😂','en',10),('haha','😂','en',10),('funny','😂','en',9),
('hi','👋','en',10),('hello','👋','en',10),('hey','👋','en',10),
('bye','👋','en',10),('goodbye','👋','en',9),
('love','❤️','en',10),('heart','❤️','en',10),
('angry','😡','en',10),('mad','😡','en',9),
('sad','😢','en',10),('crying','😢','en',9),
('fire','🔥','en',10),('cool','🔥','en',9),('awesome','🔥','en',9),
('ok','👍','en',10),('okay','👍','en',10),('good','👍','en',9),
('bad','👎','en',10),('no','👎','en',9),
('thanks','🙏','en',10),('thank you','🙏','en',10),
('wow','😮','en',10),('omg','😮','en',10),
('hmm','🤔','en',10),('thinking','🤔','en',9),
('tired','😴','en',10),('sleepy','😴','en',9),
('hungry','🍕','en',9),
('party','🎉','en',10),('congrats','🎉','en',10),('birthday','🎂','en',10)
ON CONFLICT DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS emoji_keywords CASCADE;
-- +goose StatementEnd
-- name: GetEmojiByKeyword :many
-- Ищет эмодзи по тексту (для inline suggestions при вводе обычного текста)
-- Возвращает уникальные эмодзи, отсортированные по весу
SELECT DISTINCT ON (emoji) emoji, MAX(weight) as weight
FROM emoji_keywords
WHERE keyword ILIKE '%' || $1 || '%'
GROUP BY emoji
ORDER BY emoji, weight DESC
LIMIT 8;

-- name: AddEmojiKeyword :exec
INSERT INTO emoji_keywords (keyword, emoji, lang, weight)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;
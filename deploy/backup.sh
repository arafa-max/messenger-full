#!/bin/sh
# deploy/backup.sh
# Запускается как cron внутри контейнера postgres
# Crontab: 0 3 * * * /backup.sh
# Хранит бэкапы 7 дней, потом удаляет старые

set -e

BACKUP_DIR="/backups"
DATE=$(date +%Y-%m-%d_%H-%M-%S)
FILENAME="messenger_${DATE}.sql.gz"
KEEP_DAYS=7

echo "▶ Backup started: $FILENAME"

# Дамп + сжатие
PGPASSWORD="${POSTGRES_PASSWORD}" pg_dump \
  -h postgres \
  -U "${POSTGRES_USER}" \
  -d "${POSTGRES_DB}" \
  --no-owner \
  --no-acl \
  | gzip > "${BACKUP_DIR}/${FILENAME}"

SIZE=$(du -sh "${BACKUP_DIR}/${FILENAME}" | cut -f1)
echo "✅ Backup done: ${FILENAME} (${SIZE})"

# Удаляем старые бэкапы
find "${BACKUP_DIR}" -name "messenger_*.sql.gz" -mtime +${KEEP_DAYS} -delete
echo "🗑️  Old backups cleaned (>${KEEP_DAYS} days)"

# Список текущих бэкапов
echo "📦 Current backups:"
ls -lh "${BACKUP_DIR}"/messenger_*.sql.gz 2>/dev/null || echo "  (none)"
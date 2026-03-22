# messenger-services

Python AI microservices for messenger-full.

## Services

| Service     | Port | Tech           | RAM   |
|-------------|------|----------------|-------|
| Whisper STT | 9300 | faster-whisper | ~500MB|
| Piper TTS   | 9301 | piper binary   | ~200MB|

> Порты 9300/9301 — не конфликтуют с MinIO (9000/9001)

## Структура

```
messenger-services/
├── whisper/
│   ├── main.py
│   ├── requirements.txt
│   └── Dockerfile
├── piper/
│   ├── main.py
│   ├── requirements.txt
│   ├── Dockerfile
│   └── models/           ← скачать вручную
│       ├── ru_RU-ruslan-medium.onnx
│       └── en_US-lessac-medium.onnx
├── start.sh              ← локальный запуск (Linux)
└── README.md

```

## Скачать модели Piper

```bash
cd piper/models

# Русский
wget https://huggingface.co/rhasspy/piper-voices/resolve/main/ru/ru_RU/ruslan/medium/ru_RU-ruslan-medium.onnx
wget https://huggingface.co/rhasspy/piper-voices/resolve/main/ru/ru_RU/ruslan/medium/ru_RU-ruslan-medium.onnx.json

# Английский
wget https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/lessac/medium/en_US-lessac-medium.onnx
wget https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/lessac/medium/en_US-lessac-medium.onnx.json
```

## Локальный запуск (Linux)

```bash
chmod +x start.sh
./start.sh
```

## Продакшн

Запускается через docker-compose автоматически.
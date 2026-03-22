#!/bin/bash
cd "$(dirname "$0")"

echo "Starting Whisper STT on port 9300..."
(cd whisper && uvicorn main:app --host 0.0.0.0 --port 9300) &
WHISPER_PID=$!

echo "Starting Piper TTS on port 9301..."
(cd piper && uvicorn main:app --host 0.0.0.0 --port 9301) &
PIPER_PID=$!

echo "All services started! Whisper PID=$WHISPER_PID, Piper PID=$PIPER_PID"

trap "kill $WHISPER_PID $PIPER_PID 2>/dev/null; exit 0" SIGINT SIGTERM
wait
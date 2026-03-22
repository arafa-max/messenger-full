from fastapi import FastAPI, HTTPException
from fastapi.responses import Response
from pydantic import BaseModel
import subprocess
import tempfile
import os

app = FastAPI()

MODELS = {
    "ru": "models/ru_RU-ruslan-medium.onnx",
    "en": "models/en_US-lessac-medium.onnx",
    # kk убран — модель не существует в открытом доступе.
    # При обнаружении казахского текста fallback на "ru"
}

DEFAULT_LANG = "ru"
PIPER_BIN    = "piper"

class TTSRequest(BaseModel):
    text:     str
    language: str = None
    speed:    float = 1.0

def detect_language(text: str) -> str:
    if not text:
        return DEFAULT_LANG
    total    = len(text)
    kk_chars = sum(1 for c in text if c in 'ӘәҒғҚқҢңҰұҮүҺһІі')
    ru_chars = sum(1 for c in text if '\u0400' <= c <= '\u04FF')
    # kk → fallback на ru (нет модели), ru если >30% кириллицы
    if kk_chars / total > 0.05 or ru_chars / total > 0.3:
        return "ru"
    return "en"

@app.post("/tts")
async def tts(req: TTSRequest):
    if not req.text:
        raise HTTPException(status_code=400, detail="text is required")
    if len(req.text) > 1000:
        raise HTTPException(status_code=400, detail="text too long (max 1000 chars)")
    if not (0.5 <= req.speed <= 2.0):
        raise HTTPException(status_code=400, detail="speed must be between 0.5 and 2.0")

    lang  = req.language or detect_language(req.text)
    model = MODELS.get(lang, MODELS[DEFAULT_LANG])

    if not os.path.exists(model):
        # Пробуем дефолтную модель
        model = MODELS[DEFAULT_LANG]
        if not os.path.exists(model):
            raise HTTPException(status_code=503, detail="TTS model not found")

    with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as tmp:
        tmp_path = tmp.name

    try:
        proc = subprocess.run(
            [PIPER_BIN, "--model", model, "--output_file", tmp_path,
             "--length_scale", str(round(1.0 / req.speed, 3))],
            input=req.text.encode("utf-8"),
            capture_output=True,
            timeout=30,
        )
        if proc.returncode != 0:
            raise HTTPException(
                status_code=500,
                detail=f"piper failed: {proc.stderr.decode()}"
            )

        with open(tmp_path, "rb") as f:
            audio = f.read()

        return Response(content=audio, media_type="audio/wav")
    except subprocess.TimeoutExpired:
        raise HTTPException(status_code=504, detail="TTS timeout")
    finally:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)

@app.get("/languages")
def languages():
    return {
        "languages": {
            lang: os.path.exists(path)
            for lang, path in MODELS.items()
        }
    }

@app.get("/health")
def health():
    models_ok = any(os.path.exists(p) for p in MODELS.values())
    return {"status": "ok" if models_ok else "degraded", "models": {
        lang: os.path.exists(path) for lang, path in MODELS.items()
    }}
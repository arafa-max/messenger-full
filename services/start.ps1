Write-Host "Starting Whisper STT on port 9300..."
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd $PSScriptRoot\whisper; uvicorn main:app --host 0.0.0.0 --port 9300"

Write-Host "Starting Piper TTS on port 9301..."
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd $PSScriptRoot\piper; uvicorn main:app --host 0.0.0.0 --port 9301"

Write-Host "Done!"
Write-Host "Whisper: http://localhost:9300/health"
Write-Host "Piper:   http://localhost:9301/health"
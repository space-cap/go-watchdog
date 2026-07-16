@echo off
echo ===================================================
echo [Go-Watchdog] Uploading watchdog-server to Ubuntu...
echo ===================================================

if not exist bin\watchdog-server (
    echo [ERROR] watchdog-server binary not found in bin directory!
    echo Please run build.bat first to compile the server.
    pause
    exit /b 1
)

set KEY_PATH="h:\ssh-key-2026-06-19.key"
set SERVER_IP=140.245.64.172
set SERVER_USER=ubuntu
set TARGET_DIR=~/go-watchdog

echo 1. Stopping remote watchdog-server...
ssh -n -i %KEY_PATH% %SERVER_USER%@%SERVER_IP% "pkill -f watchdog-server"

echo 2. Uploading watchdog-server binary via SCP...
scp -i %KEY_PATH% bin\watchdog-server %SERVER_USER%@%SERVER_IP%:%TARGET_DIR%/watchdog-server

if %ERRORLEVEL% neq 0 (
    echo [ERROR] SCP transfer failed!
    pause
    exit /b 1
)

echo 3. Starting remote watchdog-server...
ssh -n -i %KEY_PATH% %SERVER_USER%@%SERVER_IP% "chmod +x %TARGET_DIR%/watchdog-server && cd %TARGET_DIR% && nohup ./watchdog-server -config config.json > server.log 2>&1 < /dev/null &"

echo ===================================================
echo [SUCCESS] watchdog-server uploaded and restarted!
echo ===================================================
pause

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

echo 1. Stopping the remote server...
:: [방법 A] systemd 서비스를 사용하는 경우 (권장)
:: ssh -i %KEY_PATH% %SERVER_USER%@%SERVER_IP% "sudo systemctl stop go-watchdog"

:: [방법 B] 만약 nohup 백그라운드로 직접 구동 중인 경우
ssh -i %KEY_PATH% %SERVER_USER%@%SERVER_IP% "pkill -f watchdog-server"

echo 2. Uploading new binary via SCP...
scp -i %KEY_PATH% bin\watchdog-server %SERVER_USER%@%SERVER_IP%:%TARGET_DIR%/watchdog-server

if %ERRORLEVEL% neq 0 (
    echo [ERROR] SCP transfer failed!
    pause
    exit /b 1
)

echo 3. Starting the remote server...
:: [방법 A] systemd 서비스를 사용하는 경우 (권장)
:: ssh -i %KEY_PATH% %SERVER_USER%@%SERVER_IP% "chmod +x %TARGET_DIR%/watchdog-server && sudo cp %TARGET_DIR%/watchdog-server /opt/go-watchdog/watchdog-server && sudo systemctl start go-watchdog"

:: [방법 B] 만약 nohup 백그라운드로 직접 구동 중인 경우
ssh -i %KEY_PATH% %SERVER_USER%@%SERVER_IP% "chmod +x %TARGET_DIR%/watchdog-server && cd %TARGET_DIR% && nohup ./watchdog-server -config config.json > server.log 2>&1 &"

echo ===================================================
echo [SUCCESS] watchdog-server uploaded and restarted!
echo ===================================================
pause


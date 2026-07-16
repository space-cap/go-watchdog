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

echo Uploading watchdog-server...
scp -i "h:\ssh-key-2026-06-19.key" bin\watchdog-server ubuntu@140.245.64.172:~/go-watchdog/watchdog-server

if %ERRORLEVEL% neq 0 (
    echo [ERROR] SCP transfer failed!
    pause
    exit /b 1
)

echo ===================================================
echo [SUCCESS] watchdog-server uploaded successfully!
echo ===================================================
pause

@echo off
echo ===================================================
echo [Go-Watchdog] Building binaries...
echo ===================================================

REM Create bin directory if it doesn't exist
if not exist bin mkdir bin

echo 1. Building watchdog-server (for Linux)...
set GOOS=linux
set GOARCH=amd64
go build -o bin/watchdog-server ./server
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Failed to build watchdog-server!
    goto error
)

echo 2. Building watchdog-agent (for Windows)...
set GOOS=windows
set GOARCH=amd64
go build -o bin/watchdog-agent.exe ./agent
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Failed to build Windows agent!
    goto error
)

echo 3. Building watchdog-agent (for Linux)...
set GOOS=linux
set GOARCH=amd64
go build -o bin/watchdog-agent ./agent
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Failed to build Linux agent!
    goto error
)

:: Restore environment variables
set GOOS=
set GOARCH=

echo ===================================================
echo [SUCCESS] Build completed successfully!
echo Binaries are located in the "bin" directory:
echo   - bin/watchdog-server (Linux Server)
echo   - bin/watchdog-agent.exe (Windows Agent)
echo   - bin/watchdog-agent (Linux Agent)
echo ===================================================
pause
exit /b 0

:error
set GOOS=
set GOARCH=
echo ===================================================
echo [FAILURE] Build failed! Please check Go SDK status.
echo ===================================================
pause
exit /b 1

@echo off
rem Start ARTEX and handle restart requests from the built-in updater.
rem Usage: start.bat [-addr :9000]
setlocal enabledelayedexpansion
cd /d "%~dp0"

set "BIN=artex.exe"
if not exist "%BIN%" (
    echo [artex] Executable not found: %BIN% 1>&2
    exit /b 1
)

set "RESTART_CODE=75"
set "MAX_DELAY=60"
set /a delay=1

:loop
"%BIN%" %*
set "code=!ERRORLEVEL!"

if "!code!"=="0" (
    echo [artex] Exited normally
    exit /b 0
)

if "!code!"=="%RESTART_CODE%" (
    echo [artex] Restart requested
    set /a delay=1
    goto loop
)

echo [artex] Exit code !code!; restarting in !delay!s 1>&2
set /a pings=!delay!+1
ping -n !pings! 127.0.0.1 >nul 2>&1
set /a delay=!delay!*2
if !delay! gtr %MAX_DELAY% set /a delay=%MAX_DELAY%
goto loop

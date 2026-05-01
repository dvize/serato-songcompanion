@echo off
title Serato Companion
cd /d "%~dp0"

where node >nul 2>&1
if %errorlevel% neq 0 (
    echo Node.js is required but not found.
    echo Download it from https://nodejs.org and re-run this script.
    pause
    exit /b 1
)

if not exist "electron\node_modules" (
    echo Installing dependencies (one-time)...
    cd electron && npm install && cd ..
)

echo Starting Serato Companion...
cd electron && npm start

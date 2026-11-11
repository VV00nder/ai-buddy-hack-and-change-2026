@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo === %date% %time% === > docker-run.log
docker version >> docker-run.log 2>&1
if errorlevel 1 (
  echo DOCKER_NOT_RUNNING >> docker-run.log
  echo Docker Desktop не запущен. Запустите его и повторите.
  pause
  exit /b 1
)
echo Собираю и запускаю AI-buddy, это займёт 3-6 минут...
docker compose up --build -d >> docker-run.log 2>&1
if errorlevel 1 (echo COMPOSE_FAILED >> docker-run.log) else (echo COMPOSE_OK >> docker-run.log)
docker compose ps >> docker-run.log 2>&1
echo Готово. Откройте http://localhost:8080
timeout /t 15

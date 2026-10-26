@echo off
chcp 65001 >nul
rem Builds QQMusic.exe into build\ and starts it.
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 没有找到 Go，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)

rem The running exe cannot be overwritten: close it first, and wait until
rem it has exited, which lets go of the file.
tasklist /fi "imagename eq QQMusic.exe" 2>nul | find /i "QQMusic.exe" >nul
if errorlevel 1 goto build
echo 正在关闭运行中的 QQMusic.exe ...
taskkill /im QQMusic.exe /f >nul 2>nul
set /a tries=0
:wait
tasklist /fi "imagename eq QQMusic.exe" 2>nul | find /i "QQMusic.exe" >nul
if errorlevel 1 goto build
set /a tries+=1
if %tries% geq 10 (
    echo [错误] 无法关闭运行中的 QQMusic.exe，请手动关闭后重试
    pause
    exit /b 1
)
rem Waits a second; timeout cannot when input is redirected.
ping -n 2 127.0.0.1 >nul
goto wait

:build

echo 正在编译 QQMusic.exe ...
set CGO_ENABLED=0
rem Puts resources\icon.ico into the exe, for Explorer, the taskbar and Alt+Tab.
go run ./cmd/genicon -syso
if errorlevel 1 (
    echo [错误] 生成图标资源失败
    pause
    exit /b 1
)
go build -ldflags "-H=windowsgui -s -w" -o build\QQMusic.exe ./cmd/qqmusic
if errorlevel 1 (
    echo [错误] 编译失败
    pause
    exit /b 1
)

echo 编译完成，正在启动...
rem Serves a page of the app's memory over time, and net/http/pprof.
set QQMUSIC_PPROF=127.0.0.1:6060
echo 内存监控页面: http://127.0.0.1:6060/mem
start "" "%~dp0build\QQMusic.exe"
endlocal

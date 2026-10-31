@echo off
chcp 65001 >nul
rem Develops the app: go tool mygo dev rebuilds and restarts it when the Go
rem code, mygo.json, the resources or the icons change. The app it runs is
rem .mygo\dev\windows-amd64\QQMusic Dev.exe.
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 没有找到 Go，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)

rem The development app and the packaged one read the same files of
rem %AppData%\QQMusicGo: run one at a time, or they overwrite each other's
rem settings and queue.
call :close QQMusicGo.exe
if errorlevel 1 exit /b 1
call :close "QQMusic Dev.exe"
if errorlevel 1 exit /b 1

echo 正在启动开发模式，改动代码后自动重新编译并重启...
set CGO_ENABLED=0
rem Serves a page of the app's memory over time, and net/http/pprof.
set QQMUSIC_PPROF=127.0.0.1:6060
echo 内存监控页面: http://127.0.0.1:6060/mem
echo 本窗口：输入 r 回车重新编译，q 回车退出。
go tool mygo dev
if errorlevel 1 (
    echo.
    echo [错误] 开发模式异常退出，请看上面的报错
    pause
)
endlocal
exit /b 0

:close
rem Kills the running image %~1 and waits until it is gone: it holds its own
rem exe, and the files of %AppData%\QQMusicGo.
tasklist /fi "imagename eq %~1" 2>nul | find /i "%~1" >nul
if errorlevel 1 exit /b 0
echo 正在关闭运行中的 %~1 ...
taskkill /im "%~1" /f >nul 2>nul
set /a tries=0
:close.wait
tasklist /fi "imagename eq %~1" 2>nul | find /i "%~1" >nul
if errorlevel 1 exit /b 0
set /a tries+=1
if %tries% geq 10 (
    echo [错误] 无法关闭 %~1，请手动关闭后重试
    pause
    exit /b 1
)
rem Waits a second; timeout cannot when input is redirected.
ping -n 2 127.0.0.1 >nul
goto close.wait

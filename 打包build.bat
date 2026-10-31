@echo off
chcp 65001 >nul
rem Packages QQMusic into build\release: the portable exe, which runs as it
rem is, and the installer, both named with the version of mygo.json.
rem A version argument, as in "打包build.bat 0.2.0", sets it first.
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 没有找到 Go，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)

set "NEWVER=%~1"
if "%NEWVER%"=="" goto version
echo %NEWVER%| findstr /r "^[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$" >nul
if errorlevel 1 (
    echo [错误] 版本号格式应为 x.y.z，例如 0.2.0
    pause
    exit /b 1
)
rem [char]34 is a double quote, which cmd would take for its own.
powershell -NoProfile -Command "$q=[char]34; $t=[IO.File]::ReadAllText('mygo.json'); $t=$t -replace ('('+$q+'version'+$q+'\s*:\s*'+$q+')[^'+$q+']*'), ('${1}'+'%NEWVER%'); [IO.File]::WriteAllText('mygo.json', $t)"
if errorlevel 1 (
    echo [错误] 写入 mygo.json 的版本号失败
    pause
    exit /b 1
)

:version
set "VERSION="
for /f "usebackq delims=" %%v in (`powershell -NoProfile -Command "(Get-Content -Raw mygo.json | ConvertFrom-Json).version"`) do set "VERSION=%%v"
if "%VERSION%"=="" (
    echo [错误] 无法从 mygo.json 读取版本号
    pause
    exit /b 1
)

echo 正在打包 QQMusicGo %VERSION% ...
set CGO_ENABLED=0
rem Builds build\windows-amd64\QQMusicGo.exe, with the icon and the version
rem information, and the NSIS installer beside it.
go tool mygo build
if errorlevel 1 (
    echo [错误] 打包失败，如果 QQMusicGo 正在运行，请关闭后重试
    pause
    exit /b 1
)

set "OUT=build\windows-amd64"
set "REL=build\release"
set "PORTABLE=QQMusic_v%VERSION%_Portable.exe"
set "SETUP=QQMusic_v%VERSION%_Setup.exe"
if not exist "%REL%" mkdir "%REL%"
rem The pictures are embedded, so the exe alone is the portable app.
copy /y "%OUT%\QQMusicGo.exe" "%REL%\%PORTABLE%" >nul
if errorlevel 1 goto copyfailed
copy /y "%OUT%\qqmusic-setup-%VERSION%-amd64.exe" "%REL%\%SETUP%" >nul
if errorlevel 1 goto copyfailed

echo.
echo 打包完成：%~dp0%REL%
echo   启动版  %PORTABLE%
echo   安装版  %SETUP%
pause
endlocal
exit /b 0

:copyfailed
echo [错误] 复制到 %REL% 失败，如果启动版正在运行，请关闭后重试
pause
exit /b 1

@echo off
setlocal EnableDelayedExpansion

rem Usage:  show_tree.bat [folder]            - print to console
rem         show_tree.bat [folder] > out.txt  - save to a file
rem Skips:  \.git\*   \dbc\*   *_test.go

set "ROOT=%~1"
if "%ROOT%"=="" set "ROOT=%CD%"
pushd "%ROOT%" || (echo Folder not found: %ROOT% & exit /b 1)
set "ROOT=%CD%"

rem --- compute length of ROOT so we can cut it off each path ---
set "S=!ROOT!"
set "LEN=0"
for %%P in (4096 2048 1024 512 256 128 64 32 16 8 4 2 1) do (
  if not "!S:~%%P,1!"=="" (
    set /a "LEN+=%%P"
    set "S=!S:~%%P!"
  )
)
set /a LEN+=1

for /r %%F in (*) do (
  set "FULL=%%F"
  set "REL=!FULL:~%LEN%!"
  set "SKIP="
  if /i "!REL:~0,6!"=="\.git\" set "SKIP=1"
  if /i "!REL:~0,5!"=="\dbc\" set "SKIP=1"
  if /i "!REL:~-8!"=="_test.go" set "SKIP=1"
  if /i "%%~fF"=="%~f0" set "SKIP=1"
  if not defined SKIP (
    echo ================================================================
    echo FILE: !REL:~1!
    echo ================================================================
    type "%%F"
    echo.
    echo.
  )
)

popd
endlocal

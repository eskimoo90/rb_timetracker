@echo off
set datetime=%date:~-4,4%%date:~-7,2%%date:~-10,2%
set timevar=%time:~0,2%%time:~3,2%%time:~6,2%
set version=%datetime%_%timevar%
echo %version% > version.txt
go build -ldflags "-H=windowsgui -X 'main.Version=%version%'" .
@echo off
set PATH=C:\mingw-ucrt\mingw64\bin;%PATH%
cd /d d:\PROJECTS\AI_BASED_PROJECTS\Kestrel
kestrel.exe -config config.yaml > kestrel-server.log 2>&1

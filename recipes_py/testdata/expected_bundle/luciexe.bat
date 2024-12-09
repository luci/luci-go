@echo off
set PYTHONPATH="%~dp0\recipe_engine"
call vpython3.bat -u "%~dp0\recipe_engine\recipe_engine\main.py" ^
 --package "%~dp0\example_repo\infra\config\recipes.cfg" ^
 --proto-override "%~dp0\_pb3" ^
 -O dep_repo=%~dp0\dep_repo ^
 luciexe %*

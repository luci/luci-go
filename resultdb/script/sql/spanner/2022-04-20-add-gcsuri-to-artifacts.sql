-- Schema update script to add GcsUri to Artifacts.
--
-- Last statement intentionally does not end with ';' otherwise it won't work
-- when passed in through ddl update --ddl-file.
ALTER TABLE Artifacts ADD COLUMN GcsURI STRING(MAX)

-- Schema update script to reduce VariantHash column from 64 characters
-- to 16 for b/226484358.
ALTER TABLE TestResults ALTER COLUMN VariantHash STRING(16) NOT NULL;
ALTER TABLE TestExonerations ALTER COLUMN VariantHash STRING(16) NOT NULL;
ALTER TABLE UniqueTestVariants ALTER COLUMN VariantHash STRING(16) NOT NULL;

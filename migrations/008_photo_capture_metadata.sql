-- 008_photo_capture_metadata.sql: normalized, non-sensitive EXIF capture metadata.
ALTER TABLE photo_index ADD COLUMN camera_make TEXT;
ALTER TABLE photo_index ADD COLUMN camera_model TEXT;
ALTER TABLE photo_index ADD COLUMN lens_model TEXT;
ALTER TABLE photo_index ADD COLUMN exposure_time_s REAL;
ALTER TABLE photo_index ADD COLUMN f_number REAL;
ALTER TABLE photo_index ADD COLUMN iso INTEGER;
ALTER TABLE photo_index ADD COLUMN focal_length_mm REAL;
ALTER TABLE photo_index ADD COLUMN exposure_bias_ev REAL;
ALTER TABLE photo_index ADD COLUMN metadata_version INTEGER NOT NULL DEFAULT 0;

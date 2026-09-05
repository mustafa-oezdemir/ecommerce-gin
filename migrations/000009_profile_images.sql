ALTER TABLE users
  ADD COLUMN profile_image_filename VARCHAR(64) NOT NULL DEFAULT '' AFTER last_name;

UPDATE users
SET first_name = CASE
      WHEN first_name = '' THEN SUBSTRING_INDEX(TRIM(name), ' ', 1)
      ELSE first_name
    END,
    last_name = CASE
      WHEN last_name <> '' THEN last_name
      WHEN LOCATE(' ', TRIM(name)) > 0 THEN TRIM(SUBSTRING(TRIM(name), LOCATE(' ', TRIM(name)) + 1))
      ELSE TRIM(name)
    END
WHERE first_name = '' OR last_name = '';

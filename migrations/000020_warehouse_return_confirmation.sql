ALTER TABLE return_requests
  ADD COLUMN warehouse_return_confirmed_at DATETIME(3) NULL AFTER status;

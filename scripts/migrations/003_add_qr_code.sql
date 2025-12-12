-- Add qr_code column to urls table
ALTER TABLE urls ADD COLUMN IF NOT EXISTS qr_code TEXT;

COMMENT ON COLUMN urls.qr_code IS 'Base64-encoded PNG QR code image (data URI format)';

-- Identity moves to auth-svc's `identity` database (M2). otp-api keeps only OTP tables.
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS tenants;

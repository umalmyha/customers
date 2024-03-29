CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS CUSTOMERS(
    ID UUID DEFAULT uuid_generate_v4(),
    FIRST_NAME VARCHAR(200) NOT NULL,
    LAST_NAME VARCHAR(200) NOT NULL,
    MIDDLE_NAME VARCHAR(200),
    EMAIL VARCHAR(250) NOT NULL,
    IMPORTANCE INT DEFAULT 0,
    INACTIVE BOOLEAN DEFAULT FALSE,
    PRIMARY KEY(ID)
);

CREATE TABLE IF NOT EXISTS USERS(
    ID UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    EMAIL VARCHAR(255) NOT NULL UNIQUE,
    PASSWORD_HASH VARCHAR(255) NOT NULL
);

CREATE TABLE IF NOT EXISTS REFRESH_TOKENS(
    ID UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    USER_ID UUID REFERENCES USERS(ID) ON DELETE CASCADE,
    FINGERPRINT VARCHAR(255) NOT NULL,
    EXPIRES_IN INT NOT NULL,
    CREATED_AT TIMESTAMP WITH TIME ZONE NOT NULL
);

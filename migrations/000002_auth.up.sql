CREATE TABLE USERS(
    ID UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    EMAIL VARCHAR(255) NOT NULL UNIQUE,
    PASSWORD_HASH VARCHAR(255) NOT NULL
);

CREATE TABLE REFRESH_TOKENS(
    ID UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    USER_ID UUID REFERENCES USERS(ID) ON DELETE CASCADE,
    FINGERPRINT VARCHAR(255) NOT NULL,
    EXPIRES_IN INT NOT NULL,
    CREATED_AT TIMESTAMP WITH TIME ZONE NOT NULL
);
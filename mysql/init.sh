#!/bin/bash -ex

mysql -u root -p'password' -e "CREATE DATABASE IF NOT EXISTS \`ddfeed\`;"
mysql -u root -p'password' -e "
USE \`ddfeed\`;
CREATE TABLE IF NOT EXISTS post (
  id INT AUTO_INCREMENT PRIMARY KEY,
  body TEXT
);
CREATE TABLE IF NOT EXISTS comment (
    id INT AUTO_INCREMENT PRIMARY KEY,
    body TEXT,
    post_id INT,
    FOREIGN KEY (post_id) REFERENCES post(id) ON DELETE CASCADE
);
CREATE INDEX idx_comment_post_id ON comment(post_id);
"
mysql -u root -p'password' -e "
UPDATE performance_schema.setup_instruments
SET ENABLED = 'YES', TIMED = 'YES'
WHERE NAME LIKE 'wait/%';

UPDATE performance_schema.setup_consumers
SET ENABLED = 'YES'
WHERE NAME LIKE 'events_waits%';
"
mysql -u root -p'password' -e "\
GRANT REPLICATION CLIENT ON *.* TO 'datadog'@'%';
GRANT PROCESS ON *.* TO 'datadog'@'%';
GRANT SELECT ON *.* TO 'datadog'@'%'; FLUSH PRIVILEGES;
"
mysql -u root -p'password' -e "\
  CREATE USER IF NOT EXISTS 'backend'@'%' IDENTIFIED BY 'password';\
  GRANT ALL PRIVILEGES ON ddfeed.* TO 'backend'@'%';\
  FLUSH PRIVILEGES;"
mysql -u root -p'password' -e "
DROP PROCEDURE IF EXISTS ddfeed.enable_events_statements_consumers;
DELIMITER $$
CREATE PROCEDURE ddfeed.enable_events_statements_consumers()
    SQL SECURITY DEFINER
BEGIN
    UPDATE performance_schema.setup_consumers SET enabled='YES' WHERE name LIKE 'events_statements_%';
    UPDATE performance_schema.setup_consumers SET enabled='YES' WHERE name = 'events_waits_current';
END $$
DELIMITER ;
GRANT EXECUTE ON PROCEDURE ddfeed.enable_events_statements_consumers TO datadog@'%';
"

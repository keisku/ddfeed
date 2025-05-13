#!/bin/bash -ex

mysql -u root -p'password' -e "CREATE DATABASE IF NOT EXISTS \`ddfeed\`;"
mysql -u root -p'password' -e "
USE \`ddfeed\`;
CREATE TABLE IF NOT EXISTS post (
    id INT AUTO_INCREMENT PRIMARY KEY,
    body TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS comment (
    id INT AUTO_INCREMENT PRIMARY KEY,
    body TEXT,
    post_id INT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (post_id) REFERENCES post(id) ON DELETE CASCADE,
    INDEX idx_comment_post_id (post_id)
);
"
mysql -u root -p'password' -e "\
GRANT REPLICATION CLIENT ON *.* TO 'datadog'@'%';
GRANT PROCESS ON *.* TO 'datadog'@'%';
GRANT SELECT ON *.* TO 'datadog'@'%'; FLUSH PRIVILEGES;
CREATE SCHEMA IF NOT EXISTS datadog;
GRANT EXECUTE ON datadog.* to datadog@'%';
"
# https://docs.datadoghq.com/database_monitoring/setup_mysql/troubleshooting/#explain-plan-procedure-missing
mysql -u root -p'password' -e "
DELIMITER $$
CREATE PROCEDURE datadog.explain_statement(IN query TEXT)
    SQL SECURITY DEFINER
BEGIN
    SET @explain := CONCAT('EXPLAIN FORMAT=json ', query);
    PREPARE stmt FROM @explain;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
END $$
DELIMITER ;
GRANT EXECUTE ON PROCEDURE datadog.explain_statement TO 'datadog'@'%';
DELIMITER $$
CREATE PROCEDURE ddfeed.explain_statement(IN query TEXT)
    SQL SECURITY DEFINER
BEGIN
    SET @explain := CONCAT('EXPLAIN FORMAT=json ', query);
    PREPARE stmt FROM @explain;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
END $$
DELIMITER ;
GRANT EXECUTE ON PROCEDURE ddfeed.explain_statement TO 'datadog'@'%';
"
# https://docs.datadoghq.com/database_monitoring/setup_mysql/troubleshooting/#events-waits-current-not-enabled
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

create table users
(
    id                 int auto_increment
        primary key,
    username           varchar(255) null,
    email              varchar(255) null,
    password           varchar(255) null,
    date_registered    datetime     null,
    roles              varchar(255) null,
    avatar             varchar(255) null,
    date_last_visit    datetime     null,
    interface_language varchar(50)  null,
    interface_timezone varchar(50)  null,
    PRIMARY KEY (id),
    constraint users_pk_2
        unique (username),
    constraint users_pk_3
        unique (email)
);

CREATE TABLE custom_field_config
(
    entity_type VARCHAR(255) NOT NULL,
    config      JSON         NULL,
    PRIMARY KEY (entity_type)
);

-- Indexing remains the same syntax
CREATE INDEX custom_field_config_entity_type_index
    ON custom_field_config (entity_type);

CREATE TABLE global_settings
(
    setting_name  VARCHAR(255) NOT NULL,
    setting_value VARCHAR(255),
    PRIMARY KEY (setting_name)
);

INSERT INTO global_settings (setting_name, setting_value)
VALUES ('site_name', 'Site Name');

CREATE TABLE subforums (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NULL,
    position INT NULL,
    topic_number INT NULL,
    post_number INT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE topics (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type INT NOT NULL,
    date_created DATETIME DEFAULT CURRENT_TIMESTAMP,
    date_last_post DATETIME,
    post_number INT,
    author_user_id INT NOT NULL,
    subforum_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_topics_subforum
        FOREIGN KEY (subforum_id) REFERENCES subforums (id) ON DELETE CASCADE,
    CONSTRAINT fk_topics_user
        FOREIGN KEY (author_user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE posts (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    topic_id BIGINT UNSIGNED NOT NULL,
    author_user_id INT NOT NULL,
    date_created DATETIME DEFAULT CURRENT_TIMESTAMP,
    content TEXT NOT NULL,
    character_profile_id BIGINT UNSIGNED,
    PRIMARY KEY (id),
    CONSTRAINT fk_posts_topic
        FOREIGN KEY (topic_id) REFERENCES topics (id) ON DELETE CASCADE,
    CONSTRAINT fk_posts_user
        FOREIGN KEY (author_user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_posts_character_profile
        FOREIGN KEY (character_profile_id) REFERENCES character_profile_main (id) ON DELETE SET NULL
);

create table character_base
		(id      bigint unsigned auto_increment,
		user_id int          null,
		name    varchar(255) null,
		avatar  varchar(255) null,
		PRIMARY KEY (id),
		constraint character_base_users_id_fk
		foreign key (user_id) references users (id)
		);

create table character_profile_base
		(id      bigint unsigned auto_increment,
		character_id int          null,
		PRIMARY KEY (id),
		constraint character_profile_base_character_id_fk
		foreign key (character_id) references character_base (id)
		);

create table episode_base
		(id      bigint unsigned auto_increment,
		topic_id int          null,
		name    varchar(255) null,
		PRIMARY KEY (id),
		constraint episode_base_topics_id_fk
		foreign key (topic_id) references topics (id)
		);

create table episode_character
		(episode_id int          null,
		character_id int          null,
		);

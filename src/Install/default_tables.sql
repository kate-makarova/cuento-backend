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

CREATE TABLE categories (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NULL,
    position INT NULL
);

CREATE TABLE subforums (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    category_id INT NULL,
    name VARCHAR(255) NULL,
    description TINYTEXT NULL,
    position INT NULL,
    topic_number INT NULL,
    post_number INT NULL,
    last_post_topic_id bigint unsigned null;
    last_post_topic_name varchar(255) null;
    last_post_id bigint unsigned null;
    constraint subforums_categories_id_fk
        foreign key (category_id) references categories (id);
);

CREATE TABLE topics (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    status INT NOT NULL,
    name VARCHAR(255) NOT NULL,
    type INT NOT NULL,
    date_created DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_post_id BIGINT UNSIGNED NULL,
    date_last_post DATETIME,
    last_post_author_user_id INT NULL,
    post_number INT,
    author_user_id INT NOT NULL,
    subforum_id BIGINT UNSIGNED NOT NULL,
    CONSTRAINT fk_topics_subforum
        FOREIGN KEY (subforum_id) REFERENCES subforums (id) ON DELETE NO ACTION ,
    CONSTRAINT fk_topics_user
        FOREIGN KEY (author_user_id) REFERENCES users (id) ON DELETE NO ACTION,
    CONSTRAINT fk_topics_last_post_user
        FOREIGN KEY (last_post_author_user_id) REFERENCES users (id) ON DELETE NO ACTION,
    constraint topics_posts_id_fk
        foreign key (last_post_id) references posts (id);
);

create table character_base
		(id      bigint unsigned auto_increment primary key,
		user_id int          null,
		name    varchar(255) null,
		avatar  varchar(255) null,
        topic_id bigint unsigned null,
        character_status int default 2 not null,
		constraint character_base_users_id_fk
		foreign key (user_id) references users (id),
        constraint character_base_topics_id_fk
		foreign key (topic_id) references topics (id)
		);

create table character_profile_base
		(id      bigint unsigned auto_increment primary key,
		character_id bigint unsigned          null,
		constraint character_profile_base_character_id_fk
		foreign key (character_id) references character_base (id)  ON DELETE CASCADE
		);

CREATE TABLE posts (
                       id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
                       topic_id BIGINT UNSIGNED NOT NULL,
                       author_user_id INT NOT NULL,
                       date_created DATETIME DEFAULT CURRENT_TIMESTAMP,
                       content TEXT NOT NULL,
                       character_profile_id BIGINT UNSIGNED,
                       CONSTRAINT fk_posts_topic
                           FOREIGN KEY (topic_id) REFERENCES topics (id) ON DELETE CASCADE,
                       CONSTRAINT fk_posts_user
                           FOREIGN KEY (author_user_id) REFERENCES users (id) ON DELETE CASCADE,
                       CONSTRAINT fk_posts_character_profile
                           FOREIGN KEY (character_profile_id) REFERENCES character_profile_base (id) ON DELETE SET NULL
);

create table episode_base
		(id      bigint unsigned auto_increment primary key,
		topic_id bigint unsigned          null,
		name    varchar(255) null,
		constraint episode_base_topics_id_fk
		foreign key (topic_id) references topics (id)
		);

create table episode_character
		(episode_id bigint unsigned          null,
		character_id bigint unsigned          null,
         foreign key (episode_id) references episode_base (id),
         foreign key (character_id) references character_base (id)
		);

create table global_stats
(
    stat_name   varchar(255) null
        primary key,
    stat_number decimal      null
);

create table factions
(
    id              int          auto_increment  primary key,
    name            varchar(255) not null,
    parent_id       int          null,
    level           int          not null,
    description     text         null,
    icon            varchar(255) null,
    show_on_profile boolean      not null,
    can_be_multiple bool default FALSE null,
    root_id         int          null,
);

create table character_faction
(
    character_id bigint unsigned null,
    faction_id   int             null,
    constraint character_faction_character_base_id_fk
        foreign key (character_id) references character_base (id),
    constraint character_faction_factions_id_fk
        foreign key (faction_id) references factions (id)
);


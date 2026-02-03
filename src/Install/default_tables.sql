CREATE TABLE custom_field_config
(
    entity_type VARCHAR(255) NOT NULL,
    config      JSON         NULL,
    -- It is highly recommended to add a Primary Key for InnoDB performance
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
Feature: Push to file

    In order to protect secrets
    As a developer using summon
    I want to be able to fetch the externally
    stored secrets into the environment
    and push them to a file for use by other applications

    Scenario: A secret is missing when pushing to a file
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password-non-existent
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a non-existent secret "very/secret/db-password-non-existent"
        When I run `summon -p ./provider cat ./output/test-secrets.json`
        Then the output should contain:
            """
            Error fetching secret: very/secret/db-password-non-existent
            """
        And a file "output/test-secrets.json" should not exist

    Scenario: Default file permissions
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: test_literal
            """
        When I successfully run `rm -fr ./output`
        When I successfully run `summon -p ./provider ls -l ./output/test-secrets.json`
        Then the output should match:
            """
            -rw-------
            """
        When I successfully run `ls -ld ./output`
        Then the output should contain:
            """
            drwx------
            """
        And a file "output/test-secrets.json" should not exist

    Scenario: Multiple file groups with overlapping secrets
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/db-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password
              - path: "./output/app-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  API_KEY: !var very/secret/api-key
                  APP_ENV: production
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/db-password" with "notSoSecret"
        And a secret "very/secret/api-key" with "myApiKey123"
        When I successfully run `summon -p ./provider cat ./output/db-secrets.json`
        Then the output should match:
            """
            {"DB_PASSWORD":"notSoSecret","DB_USERNAME":"secretUsername"}
            """
        When I successfully run `summon -p ./provider cat ./output/app-secrets.json`
        Then the output should match:
            """
            {"API_KEY":"myApiKey123","APP_ENV":"production","DB_USERNAME":"secretUsername"}
            """
        And a file "output/db-secrets.json" should not exist
        And a file "output/app-secrets.json" should not exist

    Scenario: Duplicate file paths across file groups
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
              - path: "output/test-secrets.json"
                format: "json"
                secrets:
                  API_KEY: !var very/secret/api-key
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/api-key" with "myApiKey123"
        When I run `summon -p ./provider cat ./output/test-secrets.json`
        Then the output should contain:
            """
            already exists
            """
        And a file "output/test-secrets.json" should not exist

    Scenario: Pushing secrets to a YAML file
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "output/test-secrets.yaml"
                format: "yaml"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/db-password" with "notSoSecret"
        When I successfully run `summon -p ./provider cat ./output/test-secrets.yaml`
        Then the output should match:
            """
            "DB_PASSWORD": "notSoSecret"
            "DB_USERNAME": "secretUsername"
            """
        And a file "output/test-secrets.yaml" should not exist

    Scenario: Custom template for file output
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.cfg"
                template: |
                  [database]
                  username={{ secret "DB_USERNAME" }}
                  password={{ secret "DB_PASSWORD" }}
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/db-password" with "notSoSecret"
        When I successfully run `summon -p ./provider cat ./output/test-secrets.cfg`
        Then the output should contain:
            """
            [database]
            username=secretUsername
            password=notSoSecret
            """
        And a file "output/test-secrets.cfg" should not exist

    Scenario: File secrets alongside environment secrets
        Given a file named "secrets.yml" with:
            """
            DB_HOST: !var very/secret/db-host
            summon.files:
              - path: "./output/test-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password
            """
        And a secret "very/secret/db-host" with "db.example.com"
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/db-password" with "notSoSecret"
        When I successfully run `summon -p ./provider sh -c "echo $DB_HOST && cat ./output/test-secrets.json"`
        Then the output should contain:
            """
            db.example.com
            """
        And the output should contain:
            """
            {"DB_PASSWORD":"notSoSecret","DB_USERNAME":"secretUsername"}
            """
        And a file "output/test-secrets.json" should not exist

    Scenario: Custom file permissions
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.json"
                format: "json"
                permissions: 0640
                secrets:
                  DB_USERNAME: test_literal
            """
        When I successfully run `rm -fr ./output`
        When I successfully run `summon -p ./provider ls -l ./output/test-secrets.json`
        Then the output should match:
            """
            -rw-r-----
            """
        When I successfully run `ls -ld ./output`
        Then the output should contain:
            """
            drwxr-x---
            """
        And a file "output/test-secrets.json" should not exist

    Scenario: Pushing secrets to a file using a custom template
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.xml"
                format: "template"
                template: |
                  <xml>
                    <db_username>{{ secret "DB_USERNAME" }}</db_username>
                    <db_password>{{ secret "DB_PASSWORD" }}</db_password>
                  </xml>
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/db-password" with "notSoSecret"
        When I successfully run `summon -p ./provider cat ./output/test-secrets.xml`
        Then the output should contain:
            """
            <db_username>secretUsername</db_username>
            """
        And the output should contain:
            """
            <db_password>notSoSecret</db_password>
            """
        And a file "output/test-secrets.xml" should not exist

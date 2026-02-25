Feature: Push to file

    In order to protect secrets
    As a developer using summon
    I want to be able to fetch the externally
    stored secrets into the environment
    and push them to a file for use by other applications

    Scenario: Pushing secrets to a file
        Given a file named "secrets.yml" with:
            """
            summon.files:
              - path: "./output/test-secrets.json"
                format: "json"
                secrets:
                  DB_USERNAME: !var very/secret/db-username
                  DB_PASSWORD: !var very/secret/db-password
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a secret "very/secret/db-password" with "notSoSecret"
        When I successfully run `summon -p ./provider cat ./output/test-secrets.json`
        Then the output should match:
            """
            {"DB_PASSWORD":"notSoSecret","DB_USERNAME":"secretUsername"}
            """
        And a file "output/test-secrets.json" should not exist

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

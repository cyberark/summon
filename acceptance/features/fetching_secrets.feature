Feature: fetching secrets

	In order to protect secrets
	As a developer using summon
	I want to be able to fetch the externally
	stored secrets into the environment

	Scenario: Fetching a database password
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""

		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `summon -p ./provider env`
		Then the output should contain "DB_PASSWORD=notSoSecret"

	Scenario: Fetching a database username and non existent password
		Given a file named "secrets.yml" with:
			"""
			DB_USERNAME: !var very/secret/db-username
			DB_PASSWORD: !var very/secret/db-password-non-existent
			"""
		And a secret "very/secret/db-username" with "secretUsername"
		And a non-existent secret "very/secret/db-password-non-existent"
		When I run `summon -p ./provider env`
		Then the output should contain "Error fetching variable DB_PASSWORD"

    Scenario: Ignoring a specific error
        Given a file named "secrets.yml" with:
            """
            DB_USERNAME: !var very/secret/db-username
            DB_PASSWORD: !var very/secret/db-password-non-existent
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a non-existent secret "very/secret/db-password-non-existent"
        When I run `summon -p ./provider --ignore DB_PASSWORD env`
        Then the output should contain "DB_USERNAME=secretUsername"
        And the output should not contain "Error fetching variable DB_PASSWORD"
    
    Scenario: Ignoring all errors
        Given a file named "secrets.yml" with:
            """
            DB_USERNAME: !var very/secret/db-username
            DB_PASSWORD: !var very/secret/db-password-non-existent
            SOME_OTHER_VAR: !var very/secret/some-other-var-non-existent
            """
        And a secret "very/secret/db-username" with "secretUsername"
        And a non-existent secret "very/secret/db-password-non-existent"
        And a non-existent secret "very/secret/some-other-var-non-existent"
        When I run `summon -p ./provider --ignore-all env`
        Then the output should contain "DB_USERNAME=secretUsername"
        And the output should not contain "Error fetching variable DB_PASSWORD"
        And the output should not contain "Error fetching variable SOME_OTHER_VAR"

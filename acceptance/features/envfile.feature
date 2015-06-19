Feature: envfile

	In order to use some programs
	As a developer using summon
	I want to be able to have the secrets stored in a temporary key=value file
	When running the command

	Scenario: Running an env-consuming command
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""

		And a secret "very/secret/db-password" with "notSoSecret"
		When I successfully run `summon -p ./provider cat @SUMMONENVFILE`
		Then the output should contain "DB_PASSWORD=notSoSecret"

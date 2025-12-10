Feature: envfile

	In order to use some programs
	As a developer using summon
	I want to be able to have the secrets stored in a temporary key=value file
	When running the command

	Scenario: Running an env-consuming command
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			CERTIFICATE: !var very/secret/certificate
			"""

		And a secret "very/secret/db-password" with "notSoSecret"
		And a secret "very/secret/certificate" with:
			"""
			-----BEGIN CERTIFICATE-----
			CERT_DATA
			-----END CERTIFICATE-----
			"""
        When I successfully run `summon -p ./provider cat @SUMMONENVFILE`
        Then the output should contain "DB_PASSWORD=notSoSecret"
        And the output should contain "CERTIFICATE=\"-----BEGIN CERTIFICATE-----\nCERT_DATA\n-----END CERTIFICATE-----\""


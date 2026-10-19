Feature: daemon mode

	In order to pick up rotated secrets without restarting summon by hand
	As a developer using summon
	I want to run summon with --refresh and have it detect a secret
	rotation on its next poll and restart the subprocess (or, for file
	injection, just rewrite the file)

	Background:
		Given the aruba exit timeout is 20 seconds
		And the aruba io wait timeout is 20 seconds

	# NOTE: none of the steps below use aruba's built-in file-content
	# matchers ("the file ... should contain" / "should not exist") while
	# the daemon needs to stay running — those matchers call
	# stop_all_commands as a side effect, which kills the daemon's
	# subprocess (and the daemon with it) mid-scenario.

	Scenario: Restarting the subprocess with the new value when an env secret rotates
		Given a file named "secrets.yml" with:
			"""
			DB_PASSWORD: !var very/secret/db-password
			"""
		And a file named "provider" with mode "0755" and with:
			"""
			#!/bin/sh
			cat password-value.txt
			"""
		And a file named "password-value.txt" with "initialSecret"
		And I set the environment variables to:
			| variable                | value |
			| SUMMON_REFRESH_INTERVAL | 5s    |
			| SUMMON_LOG_LEVEL        | INFO  |
		When I run `summon --refresh -p ./provider sh -c 'echo "START: $DB_PASSWORD" >> start.log; sleep 60'` interactively
		And I wait for the file "start.log" to contain "START: initialSecret"

		When a file named "password-value.txt" with "rotatedSecret"
		And I wait for the file "start.log" to contain "START: rotatedSecret"
		Then the file "start.log" should already have 2 lines

		When I terminate the command started last

	Scenario: Rewriting a file secret without restarting when restart is disabled
		Given a file named "secrets.yml" with:
			"""
			summon.files:
			  - path: "./tls/cert.pem"
			    format: template
			    permissions: 0640
			    overwrite: true
			    template: |
			      {{ secret "TLS_CERT" }}
			    secrets:
			      TLS_CERT: !var very/secret/tls-cert
			"""
		And a file named "provider" with mode "0755" and with:
			"""
			#!/bin/sh
			cat cert-value.txt
			"""
		And a file named "cert-value.txt" with "initialCert"
		And I set the environment variables to:
			| variable                    | value |
			| SUMMON_REFRESH_INTERVAL     | 5s    |
			| SUMMON_RESTART_ON_ROTATION  | false |
			| SUMMON_LOG_LEVEL            | INFO  |
		When I run `summon --refresh -p ./provider sh -c 'echo START >> start.log; sleep 60'` interactively
		And I wait for the file "tls/cert.pem" to contain "initialCert"

		When a file named "cert-value.txt" with "rotatedCert"
		And I wait for the file "tls/cert.pem" to contain "rotatedCert"
		Then the file "start.log" should already have 1 line

		When I terminate the command started last

	Scenario: Restarting when a file secret rotates and restart is enabled
		Given a file named "secrets.yml" with:
			"""
			summon.files:
			  - path: "./tls/cert.pem"
			    format: template
			    permissions: 0640
			    overwrite: true
			    template: |
			      {{ secret "TLS_CERT" }}
			    secrets:
			      TLS_CERT: !var very/secret/tls-cert
			"""
		And a file named "provider" with mode "0755" and with:
			"""
			#!/bin/sh
			cat cert-value.txt
			"""
		And a file named "cert-value.txt" with "initialCert"
		And I set the environment variables to:
			| variable                | value |
			| SUMMON_REFRESH_INTERVAL | 5s    |
			| SUMMON_LOG_LEVEL        | INFO  |
		When I run `summon --refresh -p ./provider sh -c 'echo "START: $(cat tls/cert.pem)" >> start.log; sleep 60'` interactively
		And I wait for the file "start.log" to contain "START: initialCert"

		When a file named "cert-value.txt" with "rotatedCert"
		And I wait for the file "start.log" to contain "START: rotatedCert"

		When I terminate the command started last

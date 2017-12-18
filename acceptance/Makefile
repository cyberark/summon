.PHONY: cucumber

# currently built ad-hoc based on modified buncker
TEST_HARNESS = conjurinc/summon-acceptance:v1
TTY_MODE := $(shell tty -s && echo -t)

cucumber:
	docker run --rm $(TTY_MODE) -v $(PWD):/cukes -v $(PWD)/../summon:/cukes/bin/summon -w /cukes $(TEST_HARNESS) docker run --rm $(TTY_MODE) -v $(PWD):/cukes -v $(PWD)/../summon:/cukes/bin/summon -w /cukes $(TEST_HARNESS) cucumber --format junit --out reports --format pretty

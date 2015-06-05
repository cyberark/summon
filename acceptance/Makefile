.PHONY: cucumber

# currently built ad-hoc based on modified buncker
TEST_HARNESS = conjurinc/cauldron-acceptance:v1
TTY_MODE := $(shell tty -s && echo -t)

cucumber:
	docker run --rm $(TTY_MODE) -v $(PWD):/cukes -v $(PWD)/../cauldron:/cukes/bin/cauldron -w /cukes $(TEST_HARNESS) cucumber

SRCS := $(wildcard *.go)

all: compile

compile:
	go build $(SRCS)

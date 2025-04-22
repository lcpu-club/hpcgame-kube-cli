.PHONY: all clean

all: hpcgame

hpcgame: main.go
	go build -o hpcgame main.go
	@echo "Build complete."

clean:
	rm -f hpcgame
	@echo "Cleaned up build artifacts."
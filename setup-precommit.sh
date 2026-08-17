#!/bin/sh
# Installs the pre-commit hook. Run once after cloning: sh setup-precommit.sh
set -e

cd "$(git rev-parse --show-toplevel)"

cat > .git/hooks/pre-commit <<'EOF'
#!/bin/sh
set -e

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "gofmt needed on:"
	echo "$unformatted"
	echo "run: gofmt -w ."
	exit 1
fi

go vet ./...
go build ./...
EOF

chmod +x .git/hooks/pre-commit
echo "pre-commit hook installed"

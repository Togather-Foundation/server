# PostgreSQL Setup Guide

This guide covers installing and configuring PostgreSQL for local development with the Togather SEL server.

## Requirements

The SEL server requires:
- **PostgreSQL 16+** (preferably 16 or 17)
- **PostGIS** (geospatial extension)
- **pgvector** (vector similarity search)
- **pg_trgm** (fuzzy text search)
- **pg_stat_statements** (query monitoring, optional but recommended)

## Quick Start

If you already have PostgreSQL installed, verify it has the required extensions:

```bash
make db-check
```

If extensions are missing, see [Installing Extensions](#installing-extensions) below.

## Installation by Operating System

### Ubuntu/Debian

#### 1. Install PostgreSQL 16

```bash
# Add PostgreSQL apt repository
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt update

# Install PostgreSQL 16
sudo apt install postgresql-16 postgresql-client-16
```

#### 2. Install Required Extensions

```bash
# PostGIS (geospatial)
sudo apt install postgresql-16-postgis-3

# pgvector (vector similarity)
sudo apt install postgresql-16-pgvector

# pg_trgm and pg_stat_statements are included in postgresql-contrib
sudo apt install postgresql-contrib-16
```

#### 3. Start PostgreSQL

```bash
sudo systemctl start postgresql
sudo systemctl enable postgresql  # Start on boot
sudo systemctl status postgresql  # Check status
```

### macOS (Homebrew)

#### 1. Install PostgreSQL 16

```bash
brew install postgresql@16
```

#### 2. Install Required Extensions

```bash
# PostGIS
brew install postgis

# pgvector
brew tap pgvector/brew
brew install pgvector
```

#### 3. Start PostgreSQL

```bash
# Start now
brew services start postgresql@16

# Or run without background service
/opt/homebrew/opt/postgresql@16/bin/postgres -D /opt/homebrew/var/postgresql@16
```

### Fedora/RHEL/CentOS

#### 1. Install PostgreSQL 16

```bash
# Add PostgreSQL repository
sudo dnf install -y https://download.postgresql.org/pub/repos/yum/reporpms/F-$(rpm -E %fedora)-x86_64/pgdg-fedora-repo-latest.noarch.rpm

# Disable built-in PostgreSQL module
sudo dnf -qy module disable postgresql

# Install PostgreSQL 16
sudo dnf install -y postgresql16-server postgresql16-contrib
```

#### 2. Install Required Extensions

```bash
# PostGIS
sudo dnf install -y postgis34_16

# pgvector (may need to build from source)
sudo dnf install -y postgresql16-devel
cd /tmp
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make PG_CONFIG=/usr/pgsql-16/bin/pg_config
sudo make install PG_CONFIG=/usr/pgsql-16/bin/pg_config
```

#### 3. Initialize and Start PostgreSQL

```bash
# Initialize database cluster
sudo /usr/pgsql-16/bin/postgresql-16-setup initdb

# Start PostgreSQL
sudo systemctl start postgresql-16
sudo systemctl enable postgresql-16
sudo systemctl status postgresql-16
```

### Arch Linux

```bash
# Install PostgreSQL and extensions
sudo pacman -S postgresql postgis

# Install pgvector (AUR)
yay -S pgvector

# Initialize database cluster
sudo su - postgres
initdb --locale=en_US.UTF-8 -D /var/lib/postgres/data
exit

# Start PostgreSQL
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

## User Configuration

### Configure PostgreSQL User

By default, PostgreSQL uses peer authentication for local connections. This means your system username must match your database username.

#### Option 1: Use Your System Username (Recommended for Development)

Create a PostgreSQL user matching your system username:

```bash
# Switch to postgres user
sudo -u postgres psql

# In PostgreSQL shell:
CREATE USER your_username WITH CREATEDB;
ALTER USER your_username WITH SUPERUSER;
\q
```

Replace `your_username` with your actual system username (echo $USER).

Now you can run `psql` and `createdb` without sudo:

```bash
psql -d postgres -c "SELECT version();"
```

#### Option 2: Use Password Authentication

If you prefer password authentication:

1. Create a PostgreSQL user with a password:

```bash
sudo -u postgres psql
CREATE USER togather WITH PASSWORD 'your_secure_password' CREATEDB;
ALTER USER togather WITH SUPERUSER;
\q
```

2. Edit PostgreSQL's authentication config:

```bash
sudo nano /etc/postgresql/16/main/pg_hba.conf
```

Change this line:
```
local   all             all                                     peer
```

To:
```
local   all             all                                     md5
```

3. Restart PostgreSQL:

```bash
sudo systemctl restart postgresql
```

4. When running setup, you'll provide the password interactively.

### Verify Configuration

Test your PostgreSQL connection:

```bash
# Without password (peer auth)
psql -d postgres -c "SELECT version();"

# With password (if using password auth)
psql -U togather -d postgres -c "SELECT version();"
```

If successful, you should see PostgreSQL version information.

## Verify Extensions

Run the extension check:

```bash
cd /path/to/togather/server
make db-check
```

You should see output like:

```
Available extensions:
   name    | default_version | installed_version |          comment
-----------+-----------------+-------------------+---------------------------
 pg_trgm   | 1.6            |                   | text similarity measurement
 pgvector  | 0.5.1          |                   | vector data type and ivfflat access method
 postgis   | 3.4.0          |                   | PostGIS geometry and geography spatial types
```

✅ If you see all four extensions listed, you're ready!

❌ If any are missing, see [Troubleshooting](#troubleshooting) below.

## Running Setup

Once PostgreSQL is installed and configured, run the interactive setup:

```bash
cd /path/to/togather/server
make build
./server setup
```

Choose "Local PostgreSQL" and the setup will:
1. Check extensions
2. Create database
3. Install extensions
4. Run migrations
5. Create first API key

## Troubleshooting

### Extension Not Available

If `make db-check` shows extensions are missing:

**Ubuntu/Debian:**
```bash
sudo apt search postgresql-16-postgis
sudo apt search postgresql-16-pgvector
```

**macOS:**
```bash
brew search postgis
brew search pgvector
```

### Permission Denied

If you get "permission denied" errors:

1. Make sure your user has CREATEDB privilege:
```bash
sudo -u postgres psql -c "ALTER USER $USER WITH CREATEDB SUPERUSER;"
```

2. Or run commands as postgres user:
```bash
sudo -u postgres createdb togather
```

### Could Not Connect to PostgreSQL

If `psql` fails to connect:

1. Check if PostgreSQL is running:
```bash
sudo systemctl status postgresql
```

2. Check which version is running:
```bash
psql --version
pg_lsclusters  # Ubuntu/Debian
```

3. Make sure you're using the correct port (default 5432):
```bash
sudo netstat -plunt | grep postgres
```

### pg_hba.conf Authentication Issues

If you get "peer authentication failed":

1. Check your pg_hba.conf:
```bash
sudo cat /etc/postgresql/16/main/pg_hba.conf | grep -v "^#"
```

2. For development, use `trust` or `md5` for local connections:
```
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     trust
host    all             all             127.0.0.1/32            trust
```

3. Restart PostgreSQL after changes:
```bash
sudo systemctl restart postgresql
```

### macOS: postgresql@16 Not Found

If brew services can't find postgresql@16:

```bash
# Add PostgreSQL to PATH
echo 'export PATH="/opt/homebrew/opt/postgresql@16/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# Verify
which psql
psql --version
```

## Alternative: Use Docker PostgreSQL

If local installation is problematic, use Docker instead:

```bash
./server setup
# Choose "Docker" when prompted

# Or non-interactive:
./server setup --docker --non-interactive
```

Docker mode runs PostgreSQL in a container with all extensions pre-installed.

## Reference

### Default Connection Parameters

When using local PostgreSQL with peer authentication:

- **Host:** localhost
- **Port:** 5432
- **Database:** togather (created by setup)
- **User:** Your system username
- **Authentication:** peer (no password needed)

### Environment Variables

The setup command creates `.env` with:

```env
DATABASE_URL=postgresql://username@localhost:5432/togather?sslmode=disable
```

### Manual Database Creation

If you need to set up the database manually:

```bash
# Create database
createdb togather

# Enable extensions
psql -d togather -c "CREATE EXTENSION IF NOT EXISTS postgis;"
psql -d togather -c "CREATE EXTENSION IF NOT EXISTS pgvector;"
psql -d togather -c "CREATE EXTENSION IF NOT EXISTS pg_trgm;"

# Run migrations
make migrate-up
make migrate-river
```

## Additional Resources

- [PostgreSQL Official Docs](https://www.postgresql.org/docs/16/)
- [PostGIS Installation](https://postgis.net/documentation/getting_started/#installing-postgis)
- [pgvector GitHub](https://github.com/pgvector/pgvector)
- [PostgreSQL Authentication Methods](https://www.postgresql.org/docs/16/auth-methods.html)

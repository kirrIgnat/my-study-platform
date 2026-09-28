# Production deployment: Portfolio + MY.STUDY + PostgreSQL

Пример для Debian/Ubuntu. Nginx принимает внешний трафик, Go-приложение слушает только `127.0.0.1:3000`, PostgreSQL — только localhost/private network.

## 1. Установить зависимости

```bash
sudo apt update
sudo apt install -y nginx ufw certbot postgresql postgresql-contrib
```

Для сборки нужен Go 1.22+.

PostgreSQL-драйвер `github.com/lib/pq` написан на Go, поэтому системные `libpq-dev`, `pkg-config` и CGO для сборки этой версии не нужны. SQLite и `libsqlite3-dev` тоже больше не нужны.

## 2. Создать PostgreSQL-базу

Сгенерируйте отдельный длинный пароль для пользователя БД, затем:

```bash
sudo -u postgres psql
```

```sql
CREATE ROLE kirill_study WITH LOGIN PASSWORD 'CHANGE_THIS_DATABASE_PASSWORD';
CREATE DATABASE kirill_study OWNER kirill_study;
\q
```

Проверка:

```bash
psql 'postgres://kirill_study:CHANGE_THIS_DATABASE_PASSWORD@127.0.0.1:5432/kirill_study?sslmode=disable' -c 'SELECT current_database(), current_user;'
```

Не открывайте `5432/tcp` через UFW, если приложение и PostgreSQL находятся на одном VPS.

## 3. Пользователь приложения и файлы

```bash
sudo useradd --system --home /opt/cosmic-portfolio --shell /usr/sbin/nologin cosmic
sudo mkdir -p /opt/cosmic-portfolio
```

Сборка:

```bash
cd /path/to/project
go test ./...
go vet ./...
go build -trimpath -ldflags='-s -w' -o cosmic-portfolio ./cmd/server
```

Установка:

```bash
sudo cp cosmic-portfolio /opt/cosmic-portfolio/
sudo rm -rf /opt/cosmic-portfolio/templates /opt/cosmic-portfolio/static
sudo cp -r templates static /opt/cosmic-portfolio/
sudo chown -R root:root /opt/cosmic-portfolio
sudo chmod 755 /opt/cosmic-portfolio/cosmic-portfolio
```

## 4. Environment

```bash
sudo cp deploy/cosmic-portfolio.env.example /etc/cosmic-portfolio.env
sudo nano /etc/cosmic-portfolio.env
sudo chown root:root /etc/cosmic-portfolio.env
sudo chmod 600 /etc/cosmic-portfolio.env
```

Минимальный вариант:

```bash
APP_ENV=production
COOKIE_SECURE=true
PORT=3000
DATABASE_URL='postgres://kirill_study:DB_PASSWORD@127.0.0.1:5432/kirill_study?sslmode=disable'
DB_MAX_CONNS=6

ADMIN_NAME=Кирилл
ADMIN_EMAIL=kirr.ignat@gmail.com
ADMIN_PASSWORD=LONG_RANDOM_ADMIN_PASSWORD

BOOKING_TIMEZONE=Europe/Moscow
BOOKING_SCHEDULE='2=18:00,19:30,21:00;4=18:00,19:30,21:00;6=12:00,13:30,15:00'
```

`ADMIN_PASSWORD` — пароль именно от MY.STUDY, а не от почты. При первом старте он хэшируется PBKDF2-HMAC-SHA256 с уникальной солью и в PostgreSQL попадает только хэш. После проверки входа удалите `ADMIN_PASSWORD` из env и перезапустите сервис.

## 5. systemd

```bash
sudo cp deploy/cosmic-portfolio.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cosmic-portfolio
sudo systemctl status cosmic-portfolio
```

Логи:

```bash
sudo journalctl -u cosmic-portfolio -n 100 --no-pager
```

На первом старте приложение само создаст PostgreSQL-схему. Посмотреть таблицы:

```bash
sudo -u postgres psql -d kirill_study -c '\dt'
```

Посмотреть пользователей без паролей:

```bash
sudo -u postgres psql -d kirill_study -c "SELECT id,email,name,role,created_at FROM users ORDER BY created_at DESC;"
```

Убедиться, что вместо пароля хранится только хэш:

```bash
sudo -u postgres psql -d kirill_study -c "SELECT email,left(password_hash,35) AS password_hash_prefix,role FROM users;"
```

Хэш начинается с `pbkdf2_sha256$...`; исходный пароль из него не извлекается.

## 6. Nginx и HTTPS

Используйте один из готовых конфигов в `deploy/`:

- `nginx-ip-http.conf` — первичный запуск по IP;
- `nginx-domain.conf` — домен;
- `nginx-ip-https.conf` — HTTPS по IP, если ваша схема сертификатов это поддерживает.

Пример установки HTTP-конфига:

```bash
sudo mkdir -p /var/www/certbot/.well-known/acme-challenge
sudo cp deploy/nginx-ip-http.conf /etc/nginx/sites-available/cosmic-portfolio
sudo sed -i 's/YOUR_SERVER_IP/ВАШ_IP/g' /etc/nginx/sites-available/cosmic-portfolio
sudo ln -sf /etc/nginx/sites-available/cosmic-portfolio /etc/nginx/sites-enabled/cosmic-portfolio
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
```

После HTTPS:

```bash
APP_ENV=production
COOKIE_SECURE=true
```

## 7. Firewall

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status
```

Не добавляйте правило для `5432`, если PostgreSQL нужен только локальному приложению.

## 8. Проверка приложения

```bash
curl -I http://127.0.0.1:3000/profile
curl -I http://127.0.0.1:3000/study
```

Дальше проверьте сценарий: регистрация студента → один или несколько постоянных слотов → ЛК и календарь → перенос → уведомление админу → согласование → материал/ДЗ → уведомление студенту.

## 9. Backup PostgreSQL

```bash
sudo install -d -m 0700 /var/backups/kirill-study
sudo -u postgres pg_dump -Fc kirill_study > /var/backups/kirill-study/kirill_study-$(date +%F-%H%M).dump
```

Проверяйте backup не только созданием файла, но и периодическим тестовым `pg_restore` в отдельную базу.

Пример восстановления:

```bash
sudo -u postgres createdb kirill_study_restore_test
sudo -u postgres pg_restore -d kirill_study_restore_test /var/backups/kirill-study/ВАШ_BACKUP.dump
sudo -u postgres psql -d kirill_study_restore_test -c 'SELECT COUNT(*) FROM users;'
sudo -u postgres dropdb kirill_study_restore_test
```

## 10. Обновление приложения

Перед обновлением сделайте `pg_dump`, затем:

```bash
cd /path/to/project
go test ./...
go vet ./...
go build -trimpath -ldflags='-s -w' -o cosmic-portfolio ./cmd/server
sudo systemctl stop cosmic-portfolio
sudo cp cosmic-portfolio /opt/cosmic-portfolio/
sudo rm -rf /opt/cosmic-portfolio/templates /opt/cosmic-portfolio/static
sudo cp -r templates static /opt/cosmic-portfolio/
sudo chown -R root:root /opt/cosmic-portfolio
sudo systemctl start cosmic-portfolio
sudo systemctl status cosmic-portfolio
```

Миграции схемы выполняются автоматически в транзакции при старте приложения.

## Если раньше использовалась SQLite

Новая версия **не читает `study.db` автоматически**. Не удаляйте старый файл, пока не убедитесь, что нужные данные перенесены. Для пустого/тестового проекта проще начать с новой PostgreSQL-базы. Если в SQLite уже есть реальные ученики и занятия, сначала сохраните `study.db` и перенесите данные отдельной миграцией перед переключением production-трафика.

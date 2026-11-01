# MY.STUDY + self-hosted Jitsi Meet + Jibri

Интеграция в приложении ожидает отдельный публичный Jitsi origin, например `https://meet.example.com`.
MY.STUDY создаёт отдельную случайную комнату на каждое занятие и подписывает короткоживущий JWT для конкретной комнаты.
Преподаватель получает affiliation `owner`, ученик — `member`.

## Что требуется

- DNS A/AAAA для `meet.example.com`;
- Docker Engine + Docker Compose plugin;
- TLS-сертификат для `meet.example.com`;
- открытый `10000/udp` до Jitsi Videobridge;
- Jibri, если нужна серверная запись;
- достаточно CPU/RAM/диска под видеокодирование.

Jibri лучше держать на отдельной машине при регулярных/параллельных записях. Для одного преподавателя можно начать с одного Jibri.

## 1. Установить официальный docker-jitsi-meet

Используй **release archive**, а не ветку `master`.

```bash
sudo mkdir -p /opt/jitsi
cd /opt/jitsi
# скачай latest stable release с https://github.com/jitsi/docker-jitsi-meet/releases
unzip docker-jitsi-meet-*.zip
cd docker-jitsi-meet-*
cp env.example .env
./gen-passwords.sh
```

В `.env` выстави минимум:

```dotenv
CONFIG=/var/lib/my-study/jitsi
TZ=Europe/Moscow
PUBLIC_URL=https://meet.example.com
HTTP_PORT=8000
HTTPS_PORT=8443
DISABLE_HTTPS=1
ENABLE_HTTP_REDIRECT=0
ENABLE_LETSENCRYPT=0

# JWT: эти два значения должны совпасть с /etc/cosmic-portfolio.env
ENABLE_AUTH=1
ENABLE_GUESTS=0
AUTH_TYPE=jwt
JWT_APP_ID=my-study
JWT_APP_SECRET=CHANGE_TO_A_LONG_RANDOM_SECRET
JWT_ALLOW_EMPTY=0

# Роли owner/member берутся из context.user.affiliation в JWT.
# На актуальной stable-ветке token_affiliation входит в Jitsi.
ENABLE_AUTO_OWNER=0
WAIT_FOR_HOST_DISABLE_AUTO_OWNERS=true
XMPP_MUC_MODULES=token_affiliation

# Jibri
ENABLE_RECORDING=1
PROSODY_ENABLE_RECORDING_METADATA=1
JIBRI_RECORDING_DIR=/storage/recordings
JIBRI_RECORDER_USER=recorder
JIBRI_XMPP_USER=jibri
# JIBRI_RECORDER_PASSWORD и JIBRI_XMPP_PASSWORD уже создаст ./gen-passwords.sh

# Публичный адрес машины с JVB.
JVB_ADVERTISE_IPS=YOUR_PUBLIC_IP
```

Для reverse proxy web-порты Jitsi не должны торчать в Интернет. В `docker-compose.yml` у сервиса `web` привяжи их к loopback:

```yaml
ports:
  - '127.0.0.1:${HTTP_PORT}:8000'
  - '127.0.0.1:${HTTPS_PORT}:8443'
```

> В разных stable-релизах внутренний порт в compose может уже быть указан иначе. Сохрани внутреннюю часть существующей строки и добавь только `127.0.0.1:` перед host-портом.

Создай каталоги, которые требует текущий release, включая storage Jibri, затем запусти:

```bash
docker compose -f docker-compose.yml -f jibri.yml up -d
```

Проверка:

```bash
docker compose ps
docker compose logs --tail=100 jibri
docker compose logs --tail=100 jvb
```

## 2. Nginx для meet.example.com

Возьми `nginx-jitsi.conf.example` из этой папки, замени `meet.example.com`, пути сертификата и включи site.

```bash
sudo cp deploy/jitsi/nginx-jitsi.conf.example /etc/nginx/sites-available/my-study-jitsi
sudo nano /etc/nginx/sites-available/my-study-jitsi
sudo ln -s /etc/nginx/sites-available/my-study-jitsi /etc/nginx/sites-enabled/my-study-jitsi
sudo nginx -t
sudo systemctl reload nginx
```

Для WebRTC открой медиапорт:

```bash
sudo ufw allow 10000/udp
```

`8000/tcp` и `8443/tcp` наружу открывать не нужно.

## 3. Доступ MY.STUDY к готовым записям Jibri

При `CONFIG=/var/lib/my-study/jitsi` Jibri с `JIBRI_RECORDING_DIR=/storage/recordings` пишет готовые сессии на host в:

```text
/var/lib/my-study/jitsi/storage/jibri/recordings
```

Go-приложение читает `metadata.json`, находит `booking_id` (или комнату как fallback), регистрирует видео в PostgreSQL и затем отдаёт его только авторизованному преподавателю/ученику.

Приложению нужен read/write доступ, потому что преподаватель может удалить запись:

```bash
sudo apt install -y acl
sudo setfacl -Rm u:cosmic:rwx /var/lib/my-study/jitsi/storage/jibri/recordings
sudo setfacl -Rdm u:cosmic:rwx /var/lib/my-study/jitsi/storage/jibri/recordings
```

## 4. Окружение MY.STUDY

В `/etc/cosmic-portfolio.env`:

```dotenv
JITSI_PUBLIC_URL=https://meet.example.com
JITSI_APP_ID=my-study
JITSI_APP_SECRET=CHANGE_TO_THE_SAME_SECRET_AS_JWT_APP_SECRET
JIBRI_RECORDINGS_DIR=/var/lib/my-study/jitsi/storage/jibri/recordings
```

Перезапусти приложение:

```bash
sudo systemctl daemon-reload
sudo systemctl restart cosmic-portfolio
sudo systemctl status cosmic-portfolio
```

## 5. Проверка сценария

1. Открой занятие из админки.
2. Нажми **«Создать приватную конференцию»**.
3. Зайди в конференцию преподавателем.
4. Открой кабинет тестового ученика: у того же занятия должна появиться кнопка **«Войти в MY.STUDY конференцию»**.
5. Нажми преподавателем **«Начать запись»**. У обоих участников появится индикатор записи.
6. Нажми **«Остановить»** и подожди несколько секунд.
7. Обнови карточку занятия: запись появится в «Архив Jibri».
8. По умолчанию запись видит только преподаватель. Нажми **«Показать ученику»** — она появится во вкладке «Материалы» ученика.
9. **«Удалить»** удаляет и файл Jibri, и запись из БД.

## Важное про роли

MY.STUDY подписывает `context.user.affiliation=owner` для преподавателя и `member` для ученика. Для этого в Jitsi должен реально работать `token_affiliation` и быть выключен auto-owner. Если после установки ученик всё равно получает права модератора, не оставляй конфигурацию так: сначала проверь актуальный stable release, `ENABLE_AUTO_OWNER=0`, `WAIT_FOR_HOST_DISABLE_AUTO_OWNERS=true` и `XMPP_MUC_MODULES=token_affiliation`.

## Важное про запись

MY.STUDY не считает запись готовой, пока Jibri не закончил файл и не появился `metadata.json`. Поэтому сразу после «Остановить» архив может быть пуст несколько секунд. Обновление страницы запускает синхронизацию с каталогом Jibri.

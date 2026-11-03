# اتصال به دیتابیس‌های اصلی MaNGOS

برنامه اکنون از `realmd`، `character1` و `mangos1` استفاده می‌کند. اتصال اصلی
به MySQL 5.6.51 موجود در کانتینر `docker-mysqldb-1` روی `127.0.0.1:3306` است.
این کانتینر، volume و اطلاعات قبلی جایگزین یا پاک نشده‌اند.

## تنظیمات و اجرا

در `configs/realmd.conf` سه بخش مستقل داریم:

```ini
[database.login]
host = 127.0.0.1
port = 3306
name = realmd
user = root
password = 'mangos'

[database.characters]
host = 127.0.0.1
port = 3306
name = character1
user = root
password = 'mangos'

[database.world]
host = 127.0.0.1
port = 3306
name = mangos1
user = root
password = 'mangos'
```

اگر متغیر قدیمی `WOW_AUTH_DSN` یا `WOW_DB_*` را تنظیم کرده‌ای، حذفش کن تا
بر تنظیمات login فایل غلبه نکند. `realmd` فعلاً فقط به دیتابیس login نیاز دارد؛
دستور `account check` اتصال و ستون‌های موردنیاز هر سه دیتابیس را بررسی می‌کند.

```powershell
go run ./cmd/wowctl account check
go run ./cmd/wowctl realm list
go run ./cmd/wowctl realmd
```

با حساب فعلی بازی وارد شو. برای ساخت حساب جدید، با نامی که قبلاً وجود ندارد:

```powershell
go run ./cmd/wowctl account create testgo MyPassword123
```

اکانت تکراری تغییر داده نمی‌شود. حساب جدید expansion برابر 1 دارد. خود برنامه
دیگر `CREATE TABLE`، migration یا `account init` ندارد؛ مالک ساختار، دیتابیس MaNGOS است.

## قالب رمز و session

`sha_pass_hash` همان SHA1 نام و رمز uppercase است؛ خروجی کامل تابع `Credentials`
نیست. این تابع از آن hash و salt برای محاسبهٔ verifier استفاده می‌کند.
فیلدهای `s`، `v` و `sessionkey` در دیتابیس hex عدد هستند؛ adapter آن‌ها را به
بایت‌های little-endian با طول ثابت تبدیل می‌کند. حساب‌هایی با `s/v` کوتاه یا
خالی، مانند مرجع در نخستین ورود از `sha_pass_hash` مقداردهی می‌شوند. این کار
پسورد را تغییر نمی‌دهد. بررسی سلامت با `account check` هیچ داده‌ای نمی‌نویسد.

## محیط مستقل تست Docker

`all.sql` ارائه‌شده فقط ساختار دارد و دادهٔ حساب، کاراکتر یا جهان را شامل نمی‌شود.
همچنین دستور `USE` ندارد؛ ابزار آماده‌سازی، دیتابیس مقصد هر جدول را از markerهای
HeidiSQL تعیین می‌کند. SQL آماده‌شده فقط در Docker import می‌شود، نه در کد Go.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/prepare-schema.ps1 -DumpPath C:\Users\Mahdi\Desktop\all.sql
docker compose --profile sandbox up -d --wait mangos-db
$env:WOW_TEST_DSN = 'root:mangos@tcp(127.0.0.1:3308)/realmd'
go test ./... -count=1
```

Compose یک MySQL 5.6.51 جدا روی **3308** می‌سازد؛ با کانتینر اصلی 3306 تداخل
ندارد. import فقط هنگام ایجاد volume خالی انجام می‌شود. جدول‌های اصلی کاربر
برای تست پاک یا بازسازی نمی‌شوند؛ تست‌ها فقط رکوردهای موقت خودشان را حذف می‌کنند.

برای تطبیق فقط‌خواندنی حساب‌های موجود:

```powershell
$env:WOW_EXISTING_DSN = 'root:mangos@tcp(127.0.0.1:3306)/realmd'
go test ./internal/storage/realmd -run TestExistingDatabaseReadOnly -v -count=1
```

این تست hash و salt/verifier را در حافظه تطبیق می‌دهد؛ نام حساب، رمز، salt و key
را چاپ یا ذخیره نمی‌کند و session جدیدی نمی‌سازد. مقدارهای placeholder را به‌عنوان
حساب مقداردهی‌شده حساب نمی‌کند. این تست جای تست نهایی خود کلاینت بازی را نمی‌گیرد.

درایور `go-sql-driver/mysql v1.9.3` به‌طور رسمی MySQL 5.7+ را فهرست کرده، ولی
اتصال و عملیات موردنیاز این پروژه روی MySQL 5.6.51 واقعی تست شده‌اند. این نتیجه
محدود به مسیرهای آزمایش‌شده است؛ ادعای پشتیبانی رسمی درایور از 5.6 نیست.

????????? ????? `cmd/account` ? `cmd/realm` ?????? ?? ????? ????? ???? ????? ????????.

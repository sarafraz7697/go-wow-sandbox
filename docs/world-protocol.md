# فاز ۴: زیرساخت بسته‌های World

این فاز listener یا worldd ندارد؛ همهٔ بررسی‌ها با بافر حافظه و reader/writer
آزمایشی انجام می‌شوند. پروتکل فعلی مخصوص 2.4.3، build 8606 است.

## ساختار

| مسیر | مسئولیت |
|---|---|
| `internal/protocol/wire` | خواندن/نوشتن اعداد little-endian، float32، CString و packed GUID با کنترل مرز |
| `internal/protocol/world` | Packet مشترک؛ opcode عددی و بدنهٔ مستقل |
| `internal/protocol/world/tbc/frame.go` | توابع خالص تبدیل هدر کلاینت و ساخت بستهٔ سرور |
| `internal/protocol/world/tbc/stream.go` | خواندن یک بسته از io.Reader و نوشتن کامل در io.Writer؛ بدون socket یا goroutine |
| `internal/protocol/world/tbc/crypt.go` | وضعیت جداگانهٔ رمزنگاری ارسال/دریافت |
| `internal/protocol/world/tbc/opcodes` | ۱۰۶۰ ثابت و اطلاعات وضعیت/پردازش تولیدشده از مرجع |

Reader بدنه، بافر را قرض می‌گیرد؛ نباید هنگام decoding تغییر کند. خطاهای آن sticky
هستند؛ decoder هر پیام باید قبل از استفاده از داده‌ها `Err()` یا `Finish()` را بررسی کند.
`Finish()` بایت اضافه را هم رد می‌کند. برای پیام دارای tail اختیاری، decoder باید
آن را آگاهانه بخواند. Writer سقف صریح دارد و در صورت خطا هیچ نتیجهٔ قابل ارسال نمی‌دهد.

## قرارداد باینری

| جهت | هدر | length | opcode | حداکثر بدنه |
|---|---|---|---|---|
| کلاینت → سرور | ۶ بایت | uint16 big-endian، شامل ۴ بایت opcode | uint32 little-endian | ۱۰۲۳۶ بایت |
| سرور → کلاینت | ۴ بایت | uint16 big-endian، شامل ۲ بایت opcode | uint16 little-endian | ۶۵۵۳۳ بایت |

هدر سرور حتی در اندازه‌های بالاتر از 0x7fff چهار بایتی است؛ هدر توسعه‌یافتهٔ WotLK
برای این نسخه استفاده نمی‌شود. اندازهٔ خروجی بیش از ظرفیت uint16 صریحاً رد می‌شود
تا مقدار طول wrap نشود. حد opcode ورودی در framing طبق مرجع ۱۰۲۴۰ است؛ وجود opcode
در جدول و اجازهٔ وضعیت session مرحله‌ای مستقل است.

فقط هدر رمز می‌شود، نه بدنه. مشتق‌سازی کلید از session key با HMAC-SHA1 و seed مرجع
انجام می‌شود. به‌علت استفادهٔ مرجع از BigNumber، صفرهای انتهای نمایش little-endian
پیش از HMAC حذف می‌شوند؛ صفرهای ابتدای آن حفظ می‌شوند. رمز بایت‌ها با XOR و
جمع/تفریق و حفظ وضعیت بین بسته‌ها انجام می‌شود، بدون RC4 یا drop.

## قرارداد اتصال در فاز ۵

- یک Reader و یک Writer برای هر اتصال؛ هرکدام فقط یک مالک goroutine دارند.
- ReadPacket دقیقاً یک بسته می‌خواند و read-ahead ندارد. پس از AUTH_SESSION، مالک
  می‌تواند قبل از بستهٔ بعدی `EnableEncryption` را صدا بزند؛ حتی اگر TCP هر دو را یکجا آورده باشد.
- فعال‌سازی رمزنگاری فقط یک‌بار و بین عملیات مجاز است. کلید صفر رد می‌شود.
- هر خطای read یا framing برای Reader نهایی است؛ resync یا ادامه روی همان اتصال مجاز نیست.
- short write تا پایان ادامه پیدا می‌کند. خطای I/O یا zero write، Writer را برای همیشه متوقف می‌کند؛
  پس از جلو رفتن وضعیت رمزنگاری نباید بسته را دوباره فرستاد.
- خطای اعتبارسنجی بستهٔ خروجی پیش از رمزکردن رخ می‌دهد و وضعیت رمز را مصرف نمی‌کند.
- packet تحویلی Reader بدنهٔ مستقل دارد. مالک صف ارسال نباید Payload را هم‌زمان با نوشتن تغییر دهد.
- deadline، صف محدود، dispatch و handlerها در فاز ۵ اضافه می‌شوند. این لایه‌ها در فاز ۴ به شبکه وصل نشده‌اند.

## جدول opcode

`Lookup` یک کپی از metadata برمی‌گرداند. وجود اسم و status در جدول به معنی وجود
handler پیاده‌سازی‌شده در Go نیست. وضعیت‌های STATUS_AUTHED، STATUS_LOGGEDIN و غیره
از جدول C++ تولید می‌شوند؛ مقادیر PROCESS_* فقط اطلاعات مرجع هستند و تصمیمی برای
ساخت goroutine در Go نمی‌گیرند. CMSG_AUTH_SESSION در جدول session دارای STATUS_NEVER
است، چون مرجع آن را پیش از dispatch در لایهٔ اتصال پردازش می‌کند.

تولید و بررسی جدول بدون شبکه:

```powershell
node tools/generate-opcodes.mjs
node tools/generate-opcodes.mjs --check
```

ورودی‌های اصلی با اعلان مجوز خود در `tools/reference/mangos` نگهداری می‌شوند؛ ابزار
پیش از تولید، SHA-256، تعداد، یکتایی و کامل‌بودن ثبت opcodeها را بررسی می‌کند.

## آزمون‌ها

- fixtureهای دستی هدر و payload؛ اندازه‌های مرزی، CString ناقص، GUID فشرده و بایت اضافه.
- چند بستهٔ چسبیده و خواندن تک‌بایتی؛ قطع در هر نقطهٔ هدر/بدنه و خطای نوشتن ناقص.
- عبور از حالت plaintext به encrypted بین دو بسته؛ بدنهٔ دست‌نخورده و ممنوعیت reset رمز.
- سه بردار مستقل تولیدشده با Node: کلید کامل، صفرهای پرارزش، صفرهای کم‌ارزش؛ چند هدر متوالی
  برای عبور از طول کلید و کنترل جدا بودن دو جهت. این‌ها capture از کلاینت واقعی نیستند.
- fuzz برای بافر و framing؛ بدون اتصال به دیتابیس یا سوکت.

```powershell
go test ./internal/protocol/wire ./internal/protocol/world/...
go test ./internal/protocol/wire -run '^$' -fuzz FuzzReader -fuzztime 15s -parallel 2
go test ./internal/protocol/world/tbc -run '^$' -fuzz FuzzClientFraming -fuzztime 15s -parallel 2
```

## مرجع تثبیت‌شده

همهٔ رفتارها با commit `b2083578a279aac3a70a2cba8fe710a454324c4e` بررسی شده‌اند:

- [PacketCodec.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/proto/PacketCodec.cpp)
- [AuthCrypt.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/shared/Auth/AuthCrypt.cpp)
- [HMACSHA1.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/shared/Auth/HMACSHA1.cpp)
- [BigNumber.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/shared/Auth/BigNumber.cpp)

کامنت PacketCodec.h دربارهٔ هدر پنج‌بایتی عمومی است؛ شاخهٔ TBC در پیاده‌سازی cpp
مبنای این پورت است. برای نسخهٔ بعدی باید adapter تازه اضافه شود، نه تغییر پنهانی این قرارداد.

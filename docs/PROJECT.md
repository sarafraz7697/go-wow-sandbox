# مستند جامع پروژه و تفاوت‌ها با MaNGOS One

آخرین جمع‌بندی: **۲۰۲۶-۱۱-۰۳**. این سند جایگزین مستندات پراکنده و نقشهٔ فازهای قبلی است. توسعه از این پس بر اساس **دامنه و use case** انجام می‌شود، نه شمارهٔ فاز.

«پیاده‌شده» یعنی مسیر کد وجود دارد؛ «تأیید کلاینت» یعنی کاربر آن رفتار را گزارش کرده است. هیچ‌کدام به‌تنهایی معادل سازگاری کامل یا تست تمام حالت‌های شکست نیستند. در نوبت تجمیع مستندات، تست، build، اجرای سرویس یا تغییر دیتابیس انجام نشده است.

## فهرست

1. [هدف و مرجع](#هدف-و-مرجع)
2. [وضعیت و اختلاف‌ها](#وضعیت-و-اختلافها)
3. [معماری و مالکیت](#معماری-و-مالکیت)
4. [راه‌اندازی و تنظیمات](#راهاندازی-و-تنظیمات)
5. [دیتابیس و تراکنش](#دیتابیس-و-تراکنش)
6. [داده‌های ثابت](#دادههای-ثابت)
7. [قرارداد شبکه و ورود](#قرارداد-شبکه-و-ورود)
8. [حرکت، خروج و stat](#حرکت-خروج-و-stat)
9. [اعتبارسنجی و عیب‌یابی](#اعتبارسنجی-و-عیبیابی)
10. [قدم بعدی](#قدم-بعدی)
11. [منشأ و تولید کد](#منشأ-و-تولید-کد)

## هدف و مرجع

پروژه آموزشی و تفریحی برای یادگیری Go و بازسازی تدریجی مسیر login تا world کلاینت TBC است. جایگزین کامل سرور C++ یا یک سرور عمومی آماده نیست.

| منبع                                                     | قرارداد                                         |
| -------------------------------------------------------- | ----------------------------------------------- |
| [MaNGOS One](https://github.com/mangosone/server)        | رفتار و پروتکل 2.4.3 / build 8606               |
| commit `b2083578a279aac3a70a2cba8fe710a454324c4e`        | مرجع تثبیت‌شدهٔ ورودی‌های opcode و بخشی از پورت |
| `mangos/realmd@095bc797730669f228d5126f4627b7d2a2e13a8f` | مرجع تاریخی ثبت‌شده برای auth                   |
| dump ارسالی `all.sql` و دیتابیس‌های موجود                | schema واقعی MaNGOS؛ MySQL 5.6.51 در محیط توسعه |
| DBC استخراج‌شده با ابزار MaNGOS                          | دادهٔ کلاینت 2.4.3؛ extractor جدید لازم نیست    |

در بررسی‌های اخیر، سورس محلی مجاور `../server` و صفحات upstream نیز خوانده شده‌اند. سورس مجاور تغییرات محلی دارد و برابر بودن همهٔ آن با commit تثبیت‌شده ادعا نمی‌شود. برای پورت‌های بعدی، فایل/تابع و revision مرجع باید مشخص شود.

## وضعیت و اختلاف‌ها

این بخش **diff رفتاری با مرجع** است، نه diff خط‌به‌خط Git.

| دامنه                                 | آنچه وجود دارد                                                                                                                                                                                                                                                                        | محدودیت یا کار باقی‌مانده                                                                                                                                |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Auth                                  | SRP6، ذخیرهٔ session key، reconnect، ban حساب/IP، locked IP، failed_logins                                                                                                                                                                                                            | MFA، Warden، CRC، autoban و تفکیک تمام خطاهای مرجع وجود ندارند                                                                                           |
| خطای رمز                              | پاسخ خطای build 8606 و انتظار محدود برای بسته‌شدن اتصال توسط کلاینت                                                                                                                                                                                                                   | نمایش خطای صحیح در کلاینت بعد از اصلاح هنوز تأیید مستقل نشده                                                                                             |
| Realm                                 | فهرست، تعداد کاراکتر، دسترسی، build، آدرس محلی/عمومی                                                                                                                                                                                                                                  | population و وضعیت world خودکار مدیریت نمی‌شوند؛ realm ناسازگار حذف می‌شود                                                                               |
| World session                         | auth digest، header crypt، addon response، ping، صف محدود، FSM                                                                                                                                                                                                                        | admission فقط درون process؛ جایگزینی session و هماهنگی چند process کامل نیست                                                                             |
| فهرست کاراکتر                         | حساب‌محور، ظاهر، تجهیزات از cache، pet برای صفحهٔ انتخاب                                                                                                                                                                                                                              | cache بازسازی نمی‌شود؛ دادهٔ خراب ممکن است با هشدار از خروجی حذف شود                                                                                     |
| ساخت/حذف کاراکتر                      | Human Warrior، هر دو جنسیت، دادهٔ شروع و تراکنش کامل؛ حذف کاراکتر با hard delete تمام جدول‌های وابسته (character_action/aura/homebind/inventory/queststatus/reputation/skills/spell/spell_cooldown، corpse، character_pet + pet_aura/spell/cooldown، item_instance) در یک transaction | سایر race/classها هنوز برای ساخت فعال نیستند؛ نام فقط ASCII؛ realmcharacters counter در delete decrement نمی‌شود (ممکن است یکی زیاد شود)                 |
| ورود کاراکتر موجود                    | race/class معتبر در داده‌ها، هر دو جنسیت، level 1–70، map موجود در DBC                                                                                                                                                                                                                | این به معنی پشتیبانی کامل مکانیک همهٔ کلاس‌ها، instance، taxi، مرگ/احیا یا فرم نیست                                                                      |
| Inventory هنگام ورود                  | آیتم، کیف، محتویات، بانک، keyring و self update                                                                                                                                                                                                                                       | عملیات equip/move/use و ذخیرهٔ تغییر inventory پیاده نشده                                                                                                |
| حرکت                                  | جلو/عقب، strafe، چرخش، walk/run، jump/land و heartbeat                                                                                                                                                                                                                                | محلی؛ بدون broadcast، collision، terrain، سرعت‌سنجی یا ذخیرهٔ موقعیت                                                                                     |
| Logout                                | countdown، cancel، ذخیرهٔ زمان/tutorial، آزادکردن online، پاسخ completion                                                                                                                                                                                                             | آخرین اصلاح بسته‌های دیررس هنوز تأیید نشده؛ سیاست instant logout مرجع کامل نیست                                                                          |
| Stats                                 | پایه + تجهیزات، HP/Mana، armor/resistances، AP، damage/speed و enchant مستقیم                                                                                                                                                                                                         | موتور aura/talent/buff، rating و چندین modifier هنوز ناقص‌اند                                                                                            |
| Query/UI                              | self name، item template، time، played time، بعضی پیام‌های تنظیمات و chat (SAY/YELL/EMOTE/WHISPER/PARTY/GUILD/RAID/CHANNEL با echo به sender)                                                                                                                                         | social/ticket فعلاً placeholder؛ mail/raid/guild broadcast و queryهای متعدد بدون سرویس‌اند؛ whisper فقط به self و در غیر اینصورت player-not-found می‌دهد |
| guild و queryهای متعدد بدون سرویس‌اند |
| World مشترک                           | قراردادهای Intent/Event و مالکیت طراحی شده                                                                                                                                                                                                                                            | loop واقعی، map/grid، visibility، NPC، combat، spell، quest و AI وجود ندارند                                                                             |
| مدیریت                                | `wowctl account create/check` و `realm list/add`                                                                                                                                                                                                                                      | migration خودکار یا `account init` نداریم                                                                                                                |
| نسخه‌های دیگر                         | مرزبندی protocol/DBC/persistence نسخه‌محور                                                                                                                                                                                                                                            | فقط 8606 فعال است؛ سازگاری خودکار با Classic/WotLK ادعا نمی‌شود                                                                                          |

تفاوت مهم: **ساخت محدود کاراکتر** با **بارگذاری کاراکترهای موجود متنوع** دو قابلیت مستقل‌اند.

### اختلاف‌های دقیق‌تر

- Realm باید build 8606 را در `realmbuilds` داشته باشد. فیلتر نسخه با نمایش offline خانوادهٔ نسخه در مرجع یکسان نیست. subnet matching برای IPv4 انجام می‌شود؛ DNS صرفاً می‌تواند آدرس خروجی باشد.
- schema اختصاصی مانند `auth_accounts` یا ستون `client_build` به account اضافه نشده است. locale/os حساب از login به‌روز نمی‌شوند.
- محدودیت ساخت ۱۰ کاراکتر در realm و ۵۰ در حساب است؛ سیاست نام و PvP برای GM هم اعمال می‌شود. realm iconهای ۱ و ۶ در قاعدهٔ faction لحاظ می‌شوند.
- نام جدید: ۲ تا ۱۲ حرف انگلیسی، بدون سه حرف یکسان متوالی؛ Title Case، بررسی reserved و duplicate. profanity و قواعد locale کامل نیستند.
- startup برای دادهٔ ضروری ناقص سخت‌گیرانه است؛ fallback و پرکردن شکاف levelهای مرجع پیاده نشده است.
- addon: decompression محدود، حداکثر ۲۵۶ addon و نام ۲۵۵ بایتی؛ ورودی malformed رد می‌شود.
- خطای handshake world عمدتاً عمومی است؛ overspeed ping، صف ظرفیت realm و Warden وجود ندارند.
- auth account هم‌زمان در همان process world محدود می‌شود؛ server C++ هم‌زمان نباید روی character DB مشترک بنویسد.

## معماری و مالکیت

الگو: لایه‌بندی سبک با ports/adapters؛ منطق use case از packet و SQL جدا می‌شود. DDD سنگین، event bus عمومی یا goroutine برای هر opcode نداریم.

```text
cmd/
  realmd/                     entrypoint احراز هویت و Realm List
  worldd/                     entrypoint world
  wowctl/                     ابزار مدیریت
internal/
  app/                        composition roots و lifecycle منابع
  config/                     فایل، env، CLI و validation
  domain/
    account/ realm/ character/ world/
  application/
    character/create/         سرویس ساخت؛ زیرپکیج tbc برای initializer
    character/list/           فهرست حساب
    character/stats/          محاسبهٔ snapshot آمار
    worldlogin/               acquire/release پلیر
    movement/                 اعمال موقعیت محلی
    world/                    قراردادهای World/Runner/Intent/Event/Outbox
    auth/ chat/               فعلاً placeholder
  protocol/
    auth/tbc/                 SRP orchestration، reconnect، realms، codec
    client/ wire/             پروفایل نسخه و primitiveهای باینری
    world/tbc/
      frame/ crypt/ codec/    framing، header cipher، payloadهای خالص
      opcodes/ updatefields/  ثابت‌های تولیدشده
      routes/                 جایگاه آیندهٔ routeها؛ فعلاً placeholder
  storage/
    realmd/ characters/ world/ sqlutil/
    characters/mangosone8606/ قالب item_instance
  gamedata/                   Catalog immutable و loader نسخهٔ tbc
  dbc/                        parser فایل WDBC
  auth/srp/                   کتابخانهٔ SRP مستقل از transport
  transport/                  TCP lifecycle، deadline، cancellation، ظرفیت
  world/                      قرارداد مالکیت simulation؛ loop هنوز ندارد
  admin/ testclient/          ابزار مدیریت و کمک‌آزمون
configs/                      نمونه‌های conf؛ فایل واقعی ignored
tools/                        generators و schema preparation
tools/reference/mangos/       snapshotهای مرجع و manifest hash
scripts/                      بررسی مرز import
docker/                       SQL محلی آماده‌شده؛ dumpها ignored
dbc/                          دادهٔ محلی کلاینت؛ ignored
docs/PROJECT.md               مستند جامع
```

قواعد وابستگی:

- domain به protocol/storage/application/world/app وابسته نیست؛ مدل‌ها و قواعد خالص را نگه می‌دارد.
- application به storage/protocol/world/app وابسته نیست؛ interfaceها سمت مصرف‌کننده تعریف می‌شوند.
- protocol نباید SQL یا storage را import کند؛ decode → use case → encode.
- storage نباید protocol را import کند. مصرف gamedata توسط storage/world و کتابخانهٔ مشترک SRP استثناهای آگاهانه‌اند.
- `app` وابستگی‌ها را متصل می‌کند؛ cmd تنها parse، signal و خروج را مدیریت می‌کند.
- world آینده فقط مالک simulation است؛ ارتباط با session از mailbox/Intent/Event خواهد بود. قراردادها هنوز implementation نیستند.

وضعیت واقعی: auth orchestration هنوز در protocol است؛ `application/auth` سرویس استخراج‌شده ندارد. بخشی از محدودیت‌های بارگذاری هنوز داخل transaction storage است. `application/character/compat.go` و `protocol/world/tbc/split_compat.go` برای سازگاری باقی‌اند. برخی تست‌ها هنوز زیر `internal/character` هستند. این‌ها backlog پاک‌سازی‌اند.

هر اتصال یک reader، یک writer و یک حلقهٔ session دارد. Reader/Writer هرکدام مالک وضعیت cipher خود هستند. تنها حلقهٔ session state و snapshot پلیر را تغییر می‌دهد. `Handler.active` با mutex فقط admission حساب است، نه مالک state بازی. صف‌ها bounded هستند؛ shutdown اتصال را می‌بندد و منتظر workerها می‌ماند.

FSM: `Authed → LoggingIn → InWorld → LoggingOut → Authed`؛ شکست login به Authed و cancel خروج به InWorld برمی‌گردد. هر حالت زنده می‌تواند Closed شود. callback خارجی RequestTransition باید نتیجهٔ قطعی را دریافت کند؛ از خود حلقهٔ مالک فراخوانی نمی‌شود.

## راه‌اندازی و تنظیمات

فرمان‌ها از ریشهٔ repository اجرا می‌شوند. `go.mod` حداقل Go 1.24 را تعیین کرده؛ Node فقط برای generatorها لازم است. MySQL سازگار با schema و دادهٔ واقعی world لازم است. تجربهٔ قبلی MySQL 5.6.51 تنها تأیید مسیرهای آزمایش‌شده است، نه تضمین سازگاری همهٔ قابلیت‌ها.

فایل‌های واقعی:

- `configs/realmd.conf` از نمونهٔ `.example`؛ listen پیش‌فرض `127.0.0.1:3724`.
- `configs/worldd.conf` از نمونهٔ `.example`؛ listen پیش‌فرض `127.0.0.1:8085`، realm_id=1.
- فایل `.env` خودکار بارگذاری نمی‌شود. envهای process همچنان override دارند.
- تقدم: default → فایل → environment → CLI. `WOW_AUTH_DSN` override مستقیم DSN login است؛ در عیب‌یابی متغیرهای باقی‌ماندهٔ shell را بررسی کنید.

| بخش conf                | کلیدهای اصلی                                                         |
| ----------------------- | -------------------------------------------------------------------- |
| `[network]`             | listen، max_connections، auth_timeout، idle_timeout، request_timeout |
| `[database.login]`      | host، port، name، user، password                                     |
| `[database.characters]` | همان کلیدها؛ دیتابیس کاراکترهای realm                                |
| `[database.world]`      | همان کلیدها؛ دیتابیس static world                                    |
| `[world]`               | realm_id، send_queue                                                 |
| `[data]`                | dbc_path، dbc_locale، load_timeout                                   |
| `[logging]`             | level: debug / info / warn / error                                   |

نمونهٔ محلی: login=`realmd`، characters=`character1`، world=`mangos1` روی 3306. اعتبارنامه را از نمونه با محیط خود تطبیق دهید؛ این سند رمز واقعی محیط را نگه نمی‌دارد. realmd فقط به login نیاز دارد؛ worldd هر سه را مصرف می‌کند. send_queue بین ۱ و ۱۰۲۴ است؛ dbc_locale بین ۰ و ۱۵، مسیر DBC نسبت به working directory.

envهای عمومی موجود: `WOW_LISTEN`، `WOW_MAX_CONNECTIONS`، `WOW_AUTH_TIMEOUT`، `WOW_IDLE_TIMEOUT`، `WOW_REQUEST_TIMEOUT`، `WOW_LOG_LEVEL`، `WOW_DB_HOST/PORT/NAME/USER/PASSWORD` و `WOW_AUTH_DSN`. گروه DB قدیمی مربوط به login است؛ تنظیمات characters/world را در conf نگه دارید. متغیر عمومی listen می‌تواند روی هر دو سرویس اثر بگذارد.

```powershell
go run ./cmd/realmd -config configs/realmd.conf
go run ./cmd/worldd -config configs/worldd.conf
go run ./cmd/wowctl account check
go run ./cmd/wowctl realm list
go run ./cmd/wowctl account create testgo ExamplePassword123
go run ./cmd/wowctl realm add Local 127.0.0.1 8085
```

دو سرویس در ترمینال‌های جدا اجرا می‌شوند. `wowctl realmd` فرمان معتبری نیست؛ wrapperهای قدیمی cmd/account و cmd/realm در tree فعلی وجود ندارند. account create حساب تکراری را تغییر نمی‌دهد و realm add عملیات نوشتنی است. پیش از add، فهرست realm موجود را ببینید. آدرس رکورد realm باید قابل دسترسی برای کلاینت باشد؛ localhost فقط برای همان دستگاه است.

## دیتابیس و تراکنش

مالک schema، MaNGOS است؛ Go migration یا CREATE TABLE در مسیر سرویس ندارد. هر realm یک CharacterDSN دارد و `characters` ستون realm_id ندارد. برای realm دوم، اتصال مناسب لازم است.

| دیتابیس    | مصرف                                                                                                         |
| ---------- | ------------------------------------------------------------------------------------------------------------ |
| realmd     | account، banها، sessionkey، realmlist، realmcharacters                                                       |
| character1 | characters، homebind، inventory/item_instance، spells/actions/skills/reputation/tutorial و دادهٔ enumeration |
| mangos1    | playercreateinfo\*، level/class stats، XP، item_template، creature_template، reserved_name                   |

`sha_pass_hash` از نام/رمز uppercase می‌آید؛ s/v/sessionkey به شکل hex عددی مرجع ذخیره می‌شوند و adapter آن‌ها را به بایت little-endian با عرض ثابت تبدیل می‌کند. حساب‌های دارای s/v خالی/کوتاه می‌توانند هنگام login از hash موجود مقداردهی شوند. کلیدها و password نباید در لاگ درج شوند.

ساخت کاراکتر: policy سقف/نام/faction در domain است، اما facts و اجرای policy **داخل قفل و transaction** باقی می‌مانند. GET_LOCK روی connection اختصاصی، قفل account و تخصیص GUID از رقابت processهای Go جلوگیری می‌کنند. login و characters باید روی یک endpoint باشند و کاربر DB دسترسی cross-schema لازم داشته باشد. جدول‌های نوشته‌شونده باید InnoDB باشند.

transaction ساخت شامل characters/homebind، item_instance/inventory، spell/action/skills/reputation و realmcharacters است. INSERTها گروهی‌اند و موفقیت فقط بعد از commit ارسال می‌شود. نتیجهٔ commit نامعلوم خودکار retry نمی‌شود. تأخیر گزارش‌شدهٔ کاربر پس از batching از حدود ۶٫۸ ثانیه به حدود ۶۷۷ میلی‌ثانیه رسید؛ benchmark تضمین‌شده نیست.

ورود: ownership با account/guid، online flag و داده‌ها زیر transaction بررسی و پلیر رزرو می‌شود. خروج/قطع عادی: online=0، logout_time، played time، tutorial، clearing first-login و موقعیت (position_x/y/z، orientation، map، zone) در همان transaction ذخیره می‌شوند؛ موقعیت فقط وقتی player وارد world شده باشد نوشته می‌شود. **inventory، health، پول و spellهای تغییرکرده هنوز از snapshot ناقص بازنویسی نمی‌شوند.**

پس از crash ممکن است online=1 باقی بماند. startup هیچ reset سراسری نمی‌زند. دو راه recovery وجود دارد: (۱) `wowctl account character reset-online [GUID]` که دستی online flag را پاک می‌کند (بدون GUID = همه)، و (۲) AcquirePlayer اگر logout_time قدیمی‌تر از idle timeout باشد، به‌عنوان stale تشخیص داده و reclaim می‌کند (برای single-process؛ multi-process نیاز به هماهنگی خارجی دارد). سرور C++ در کنار Go روی character DB مشترک نویسنده نباشد؛ قفل قراردادی Go را رعایت نمی‌کند.

### Docker مستقل

`compose.yaml` محیط `wow-mangos-sandbox` با profile=`sandbox` و MySQL 5.6.51 روی **127.0.0.1:3308** می‌سازد. این محیط با دیتابیس اصلی 3306 متفاوت است. volume داده حفظ می‌شود؛ init SQL فقط روی volume تازه اجرا می‌شود.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/prepare-schema.ps1 -DumpPath C:\path\all.sql
docker compose --profile sandbox up -d --wait mangos-db
```

dump اولیه فقط schema داشت؛ برای startup world دادهٔ واقعی mangos1 و DBC لازم است. فایل SQL تولیدشده در docker/init محلی و ignored است. دستورهای حذف volume یا reset دیتابیس جزء راه‌اندازی عادی نیستند.

## داده‌های ثابت

Catalog پس از load کامل در startup فقط‌خواندنی است. world DB برای static queryهای هنگام بازی مصرف نمی‌شود؛ تغییر داده‌ها restart می‌خواهد. جدول MyISAM snapshot مشترک transaction ندارد، بنابراین دادهٔ static هنگام load ویرایش نشود.

DBCهای اصلی: ChrRaces، ChrClasses، CharBaseInfo، CharStartOutfit، Map، Spell، Item، SpellItemEnchantment و Faction. initializer ساخت همچنین CharSections، CharacterFacialHairStyles، SkillLine و SkillLineAbility را می‌خواند.

layoutهای اصلی TBC: Race=69/276، Class=58/232، CharBaseInfo=2/2، Outfit=41/152، Map=125/500، Spell=216/864، Item=4/16، Enchantment=34/136، Faction=53/212 (fields/record bytes). parser WDBC، حدود رشته و طول رکورد را بررسی می‌کند. loader شناسهٔ spell و cooldown metadata را می‌خواند؛ این به معنی موتور spell نیست.

display آیتم از Item.dbc تصحیح می‌شود؛ inventory type دیتابیس حفظ می‌شود. ItemDetails برای query کلاینت و stat تجهیزات در حافظه است. enchant visual و اثرهای مستقیم enchant از DBC می‌آیند. ردیف item_template با entry=0 با هشدار و شمارنده کنار گذاشته می‌شود؛ دادهٔ ضروری دیگر نباید ساختگی جایگزین شود.

## قرارداد شبکه و ورود

| جهت World       | هدر    | طول                            | opcode               | سقف body   |
| --------------- | ------ | ------------------------------ | -------------------- | ---------- |
| client → server | ۶ بایت | uint16 big-endian، شامل opcode | uint32 little-endian | ۱۰۲۳۶ بایت |
| server → client | ۴ بایت | uint16 big-endian، شامل opcode | uint16 little-endian | ۶۵۵۳۳ بایت |

فقط header رمز می‌شود. AuthCrypt TBC مشتق‌سازی HMAC-SHA1 و تبدیل stateful XOR/جمع/تفریق دارد، نه RC4. صفرهای انتهای نمایش little-endian کلید طبق رفتار BigNumber مرجع پیش از HMAC حذف می‌شوند. reader نباید read-ahead باعث عبور از مرز فعال‌سازی crypt کند. پس از خطای I/O یا مصرف cipher state، retry همان بسته مجاز نیست.

`wire.Reader` bounded و sticky-error است؛ decoder باید Finish را بررسی کند. Payload صف نباید هم‌زمان تغییر کند. سقف framing، known opcode و مجوز state سه کنترل جدا هستند.

### ترتیب کلی

1. realmd: challenge/proof → انتشار sessionkey → realm list یا reconnect.
2. worldd: AUTH_CHALLENGE → AUTH_SESSION → بررسی digest/build/ban/realm → crypt → AUTH_RESPONSE و addon.
3. character screen: CHAR_ENUM و تنظیمات اختیاری؛ سپس PLAYER_LOGIN با GUID کامل.
4. بررسی session و acquire پلیر → محاسبهٔ stat → ساخت مجموعهٔ کامل packetها → ارسال → InWorld و time sync.
5. درخواست‌های query، حرکت، ping و خروج.

بسته‌های سروری ورود فعلی به ترتیب: DUNGEON_DIFFICULTY، LOGIN_VERIFY_WORLD، ACCOUNT_DATA_TIMES، FEATURE_SYSTEM_STATUS، MOTD، SET_REST_START، BINDPOINTUPDATE، TUTORIAL_FLAGS، INITIAL_SPELLS، SEND_UNLEARN_SPELLS، ACTION_BUTTONS، INITIALIZE_FACTIONS، LOGIN_SETTIMESPEED، CLIENT_CONTROL_UPDATE، UPDATE_OBJECT خود پلیر/آیتم‌ها، INIT_WORLD_STATES و TIME_SYNC_REQ. پیشوند SMSG/MSG مطابق جدول generated است.

ROOT اجباری ورود حذف شده است. cinematic، social push کامل، guild/group updates، login spell، aura restoration، item duration و zone world-state واقعی مرجع هنوز کامل نیستند. حساب-data timestampها و بعضی لیست‌ها placeholder خالی‌اند. ترتیب همهٔ packetهای client ثابت نیست و با cache، addon و وضعیت شخصیت تغییر می‌کند.

### Router و opcodeهای جانبی

- VOICE_SESSION_ENABLE دو بایت، OPT_OUT_OF_LOOT چهار بایت، TAXI_BENCHMARK یک بایت و ACTIVE_VOICE_CHANNEL یک uint32 + CString مصرف می‌شوند؛ پاسخ موفقیت یا سرویس voice ساخته نمی‌شود.
- NAME_QUERY فقط برای self پس از load جواب دارد؛ درخواست non-self/pre-login ممکن است بی‌پاسخ بماند. ITEM_QUERY_SINGLE template را برمی‌گرداند.
- CONTACT_LIST و GMTICKET_GETTICKET پاسخ‌های placeholder دارند. MAIL_TIME، RAID_INFO، BATTLEFIELD_STATUS، MEETINGSTONE، guild bank و channelها سرویس کامل ندارند.
- opcode شناخته‌شدهٔ client/bidirectional بدون handler فقط در state مجاز metadata کنار گذاشته می‌شود و یک warning در هر اتصال/opcode می‌دهد. این صرفاً حفظ اتصال است، نه پیاده‌سازی feature؛ UI منتظر پاسخ ممکن است منتظر بماند.
- opcode ناشناخته، server-only، STATUS_NEVER و state نامجاز همچنان خطا هستند. handler موجود payload را سخت‌گیرانه decode می‌کند.
- پس از logout موفق در Authed، پیام‌های دیررس LOGGEDIN و LOGGEDIN_OR_RECENTLY_LOGGEDOUT کنار گذاشته می‌شوند؛ UPDATE_ACCOUNT_DATA مسیر مستقل خود را دارد. قبل از اولین ورود این استثنا فعال نیست. ورود موفق بعدی نشانگر recent logout را پاک می‌کند.

## حرکت، خروج و stat

### حرکت محلی

codec کل MovementInfo نسخهٔ 8606 را شامل بخش jump/pitch و tailهای وابسته بررسی می‌کند. application/movement موقعیت محدود و finite را در snapshot session اعمال می‌کند؛ جهت normalize می‌شود. فلگ‌های transport و flight خارج از محدوده‌اند. walk/run، چرخش، strafe، jump/land، heartbeat و گذار pitch/swim route دارند.

کلاینت حرکت خودش را نمایش می‌دهد؛ broadcast به دیگران وجود ندارد. محدودیت عددی مختصات جای terrain، collision، map admission، anti-cheat یا fall damage نیست. موقعیت هنگام logout در همان transaction خروج ذخیره می‌شود (فقط وقتی player وارد world شده باشد)، پس ورود بعدی از محل قبلی است. validation در application/movement است و storage فقط snapshot را می‌نویسد؛ terrain، collision و map admission هنوز وجود ندارند.

### Logout

خروج معمول countdown بیست‌ثانیه‌ای دارد؛ time sync/ping در این مدت ادامه دارند. هنگام شروع countdown، کاراکتر با partial SMSG_UPDATE_OBJECT روی UNIT_FIELD_BYTES_2 به حالت sit می‌رود و هنگام cancel (دستی یا با حرکت) دوباره stand می‌شود. جابه‌جایی یا jump countdown را لغو می‌کند؛ heartbeat بدون جابه‌جایی لغو نمی‌کند. instant logout در rest area (inn/city) هنوز پیاده نشده چون نیاز به بارگذاری areatrigger_tavern و AreaTrigger.dbc دارد. پس از save/release، LOGOUT_COMPLETE ارسال و state به Authed برمی‌گردد. بعد از آن نباید handlerی به player آزادشده دسترسی پیدا کند.

دو اصلاح متوالی انجام شد: ابتدا فقط LOGGEDIN دیررس پوشش داده شد؛ لاگ جدید نشان داد CANCEL_TRADE از LOGGEDIN_OR_RECENTLY_LOGGEDOUT است. اصلاح آخر هر دو را پوشش می‌دهد. در تثبیت چرخه logout → relogin، خطاهای غیربحرانی (send failure برای LOGOUT_COMPLETE و time sync، خطای موقت DB در release، packet دیررس با player=nil) دیگر باعث disconnect نمی‌شوند؛ release در tick بعدی retry می‌شود و handlerها با nil-check صریح از panic جلوگیری می‌کنند. خطای واقعی session change (key متفاوت از lookup) همچنان disconnect می‌دهد.

### محاسبات stat

مسیر: `application/character/stats.Calculate` → `domain/character.Stats` → codec update fields. مرجع: StatSystem.cpp، PlayerItemApply.cpp، PlayerItemEnchant.cpp، PlayerStats.cpp و DBCStructure.h.

- پنج attribute از race/class/level به‌علاوهٔ bonus تجهیزات سالم؛ itemهای backpack و bank اثر ندارند. تجهیزات شکسته کنار گذاشته می‌شوند.
- HP: base class health + direct item HP + اولین ۲۰ stamina با ضریب ۱ و مازاد با ضریب ۱۰.
- Mana: base class mana + direct item mana + اولین ۲۰ intellect با ضریب ۱ و مازاد با ضریب ۱۵؛ کلاس بدون mana این bonus را نمی‌گیرد.
- armor از تجهیزات + دو برابر agility؛ مقاومت مدارس و مقدار shield block نیز محاسبه می‌شوند.
- AP فرمول وابسته به class/level/strength/agility دارد؛ damage از template سلاح و `max(AP,0)/14 × attack seconds` می‌آید. offhand ضریب ۰٫۵ و unarmed پایهٔ ۱–۲ با زمان ۲ ثانیه دارد.
- enchant/gem بدون شرط با اثر مستقیم stat، resistance یا weapon damage اعمال می‌شود؛ proc هنگام hit اثر idle ندارد.
- فیلدهای damage min/max، speed، AP، stat bonus مثبت/منفی، armor/resistance و block صریحاً ارسال می‌شوند. HP فعلی از DB می‌آید و برای خروجی به maximum محاسبه‌شده clamp می‌شود، نه اینکه full heal شود.

**این موتور کامل stat مرجع نیست.** passive/racial spell، talent، buff ذخیره‌شده، form، equip-spell، set bonus، meta-gem شرطی، random suffix scaling، combat rating، crit/dodge/parry، mana regen، spell/healing power و ammo bonus باقی‌اند. بنابراین خصوصاً کاراکتر مجهز یا buffدار هنوز می‌تواند HP/damage متفاوت داشته باشد. warning ورود محدودیت‌ها را اعلام می‌کند؛ ادعای برابری کامل نباید از آن برداشت شود.

## اعتبارسنجی و عیب‌یابی

### شواهد موجود

| بخش                                   | شواهد و حد اعتبار                                                                                                                             |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| auth، realm، world handshake و فهرست  | کاربر با کلاینت واقعی عبور موفق را گزارش کرده                                                                                                 |
| ساخت Human Warrior                    | کاربر ساخت و نمایش و بهبود زمان را تأیید کرده                                                                                                 |
| بازآرایی تاریخی پیش از world login    | گزارش قبلی race/vet/build و تست رقابت/rollback موفق؛ کاربر refactor را هم تأیید کرده                                                          |
| بارگذاری world و کاراکترهای دیگر      | کاربر ورود با heroهای دیگر را تأیید کرده؛ صحت همهٔ stats تأیید نشده                                                                           |
| حرکت                                  | کد اضافه شده؛ گزارش صریح پوشش همهٔ حرکت‌ها در شواهد ثبت‌شده نداریم                                                                            |
| اصلاح آخر logout + تثبیت چرخه relogin | کد موجود؛ retry خطای موقت DB در release، send غیربحرانی برای LOGOUT_COMPLETE/time sync، nil-check در handlerها؛ تأیید دوبارهٔ کلاینت باقی است |
| stats جدید                            | کد موجود؛ test/build/client verification انجام نشده                                                                                           |

نتیجهٔ سبز تست‌های قدیمی به تغییرات جدید تعمیم داده نمی‌شود. طبق درخواست فعلی کاربر، نوشتن/اجرای تست تا تأیید بعدی انجام نمی‌شود. فرمان‌های زیر راهنمای آینده‌اند، نه گزارش اجرای این نوبت:

```powershell
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
```

integrationها opt-in هستند: `WOW_TEST_DSN` برای sandbox auth، `WOW_CREATION_TEST_DSN` برای ساخت روی 3308 و schemaهای موقت، `WOW_EXISTING_DSN` برای آزمون خواندنی دیتابیس موجود و `WOW_STATIC_TEST_DSN` برای خواندن دادهٔ static. نبود متغیر به معنی skip است، نه موفقیت SQL. مقدار واقعی DSN و credential را در shell محلی تنظیم کنید؛ تست نوشتنی را به DB اصلی هدایت نکنید. آزمون static فعلی مسیر DBC را با relative path قدیمی می‌سازد؛ پس از جابه‌جایی پکیج‌ها، مسیر آن باید پیش از اتکا به این integration اصلاح شود.

آزمون‌های تاریخی شامل بردار SRP/crypt، framing ناقص/چسبیده، timeout/cancellation، state transition، policy، رقابت نام و سقف و rollback درج میانی/realm count بوده‌اند. برای تغییرات اخیر logout/stat/movement/کیف‌ها پوشش تازه لازم است. `scripts/check_imports.py` فعلاً تست‌های external-package را نیز اسکن می‌کند و ممکن است وابستگی مجاز تست به protocol را نقض production بداند؛ این تفکیک باید اصلاح شود.

### لاگ‌های مفید

`[logging] level=debug` برای packet و stage؛ info برای رویدادهای اصلی، warn برای محدودیت‌ها. debug قابل غیرفعال‌سازی است و payload حساس auth نباید چاپ شود.

| علامت                           | بررسی                                                                    |
| ------------------------------- | ------------------------------------------------------------------------ |
| listener بالا نمی‌آید           | config، اتصال DB، DBC و آخرین خطای startup                               |
| static data: invalid/missing    | dataset کامل، build فایل‌ها، ساختار و locale؛ entry=0 آیتم فقط هشدار است |
| character list omitted          | GUID هشدار، race/class و نام/مختصات/equipmentCache                       |
| player login rejected           | ownership، online باقی‌مانده، at_login و داده‌های inventory/homebind     |
| unsupported opcode              | name، عدد، state و metadata؛ نداشتن handler را با payload خراب یکی نکنید |
| failure بعد از logout           | بررسی هر دو status دیررس؛ هیچ handler نباید player=nil را مصرف کند       |
| world feature not implemented   | بسته کنار گذاشته شده؛ feature هنوز وجود ندارد                            |
| stat warning یا HP متفاوت       | effectهای اعمال‌نشده، gear، aura و class/level data                      |
| مکان پس از ورود مجدد عقب برگشته | حرکت هنوز در DB ذخیره نمی‌شود؛ رفتار شناخته‌شده                          |

## قدم بعدی

**اولویت پیشنهادی: تثبیت چرخهٔ «ورود → حرکت → خروج → انتخاب کاراکتر → ورود مجدد» و سپس تکمیل stat؛ نه افزودن Mail/Raid/Guild صرفاً برای حذف warningها.**

1. پس از اجازهٔ کاربر، بررسی آخرین logout با کلاینت و آزمون packetهای دیررس، account-data و ورود مجدد؛ تأیید عدم دسترسی به player آزادشده و صحت online flag.
2. محاسبات stat را با نمونه‌های کنترل‌شده در MaNGOS تطبیق دهید: برهنه، سلاح، stamina/intellect gear، تجهیزات شکسته، enchant، کلاس mana/rage/energy و level 70. اول صحت مسیر موجود، سپس گسترش modifiers.
3. در دامنهٔ stats/spells، pipeline صریح modifier با ترتیب base value/base percent/total value/total percent بسازید؛ DBCهای rating/crit/regen و spell effect، passive/talent/buff و auraهای ذخیره‌شده را اضافه کنید. دادهٔ ناشناخته باید قابل مشاهده بماند، نه اینکه نتیجهٔ کامل ادعا شود.
4. در world/movement، مالکیت simulation واقعی، اعتبار map/position و ذخیرهٔ موقعیت تأییدشده در transaction خروج را اضافه کنید؛ قبل از broadcast چندنفره، الگوی Intent/Event و backpressure را عملی کنید.
5. عمومی‌کردن creation برای ترکیب‌های معتبر، سپس حذف/rename کاراکتر و عملیات inventory را هرکدام به‌عنوان use case مستقل توسعه دهید.
6. social/query/mail/instance، spell/combat/AI/quest و نسخه‌های دیگر پس از پایهٔ پایدار دنبال شوند.

معیار پایان مرحلهٔ پیشنهادی: چند بار ورود/خروج بدون disconnect غیرمنتظره، عدم دست‌زدن به کاراکتر حساب دیگر، آزادشدن online، نمایش stat قابل توضیح نسبت به مرجع و ثبت روشن effectهای هنوز پشتیبانی‌نشده. این معیارها هنوز کامل محقق نشده‌اند.

## منشأ و تولید کد

ورودی‌های `tools/reference/mangos/Opcodes.h` و `OpcodeTable.cpp` snapshot مرجع تثبیت‌شده‌اند؛ اعلان copyright و GPL خودشان حفظ می‌شود. ورودی generator هستند، نه یک سرور C++ دوم. `manifest.json` hashهای SHA-256 آن‌ها را نگه می‌دارد و generator قبل از تولید تطبیق می‌دهد؛ line endingهای اصلی با `.gitattributes` حفظ می‌شوند.

```powershell
node tools/generate-opcodes.mjs
node tools/generate-opcodes.mjs --check
node tools/generate-update-fields.mjs C:\path\to\pinned\UpdateFields.h
```

generated opcode metadata به‌خودی‌خود مجوز handler نیست. update fields نیز مخصوص 8606 است و نباید برای نسخهٔ دیگر دوباره استفاده شود. generatorها به‌جای ویرایش دستی خروجی مصرف شوند. تغییر مرجع نیازمند بررسی هم‌زمان commit، source، manifest و فرض‌های پروتکل است.

ارجاع‌های اصلی: [AuthCrypt](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/shared/Auth/AuthCrypt.cpp)، [OpcodeTable](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Server/OpcodeTable.cpp)، [CharacterHandler](https://github.com/mangosone/server/blob/master/src/game/WorldHandlers/CharacterHandler.cpp)، [StatSystem](https://github.com/mangosone/server/blob/master/src/game/Object/StatSystem.cpp)، [مجوز upstream](https://github.com/mangosone/server/blob/master/LICENSE).

README محلی پیشین داخل tools/reference در این بخش ادغام شده است؛ خود source snapshot، manifest و اعلان‌های مجوز حذف نمی‌شوند. این مستند مجوز جدیدی برای کد مرجع، client یا داده‌های Blizzard ایجاد نمی‌کند. README ریشه معرفی پروژه و اعتبار منابع را نگه می‌دارد؛ این فایل تنها مرجع جزئیات فنی و backlog است.

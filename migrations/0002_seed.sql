-- Hand-curated starter seed (~5 tasks per kind) covering B2-C1 grammar and vocab.
-- Bulk content comes from cmd/pregen later. Russian glosses use Russian Cyrillic;
-- Serbian content uses Serbian Cyrillic (note ћ, ђ, ј, љ, њ, џ).

-- cloze: fill the blank with the correct form
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('cloze', 3, 'cases.acc-motion',
 'Идем у ___ .',
 '{"sentence":"Идем у ___ .","blanks":1,"hint":"акузатив, кретање"}',
 '{"answers":["школу"]}',
 'Акузатив са предлозима за правац кретања (у, на).',
 'seed', 'seed-cloze-001', unixepoch()),

('cloze', 3, 'cases.loc-location',
 'Био сам у ___ цело јутро.',
 '{"sentence":"Био сам у ___ цело јутро.","blanks":1,"hint":"локатив"}',
 '{"answers":["школи"]}',
 'Локатив са предлогом у за место где се радња дешава.',
 'seed', 'seed-cloze-002', unixepoch()),

('cloze', 3, 'cases.instr',
 'Разговарам са ___ о послу.',
 '{"sentence":"Разговарам са ___ о послу.","blanks":1,"hint":"инструментал, друштво"}',
 '{"answers":["пријатељем"]}',
 'Инструментал са предлогом са за друштво.',
 'seed', 'seed-cloze-003', unixepoch()),

('cloze', 4, 'cases.gen-memory',
 'Не сећам се његовог ___ .',
 '{"sentence":"Не сећам се његовог ___ .","blanks":1,"hint":"генитив"}',
 '{"answers":["имена"]}',
 'Глагол сећати се захтева генитив.',
 'seed', 'seed-cloze-004', unixepoch()),

('cloze', 3, 'cases.dat',
 'Дао сам поклон ___ за рођендан.',
 '{"sentence":"Дао сам поклон ___ за рођендан.","blanks":1,"hint":"датив"}',
 '{"answers":["сестри"]}',
 'Датив као индиректни објекат (коме).',
 'seed', 'seed-cloze-005', unixepoch());

-- conjugation: produce the requested form
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('conjugation', 3, 'verbs.present',
 'Конјугација: ићи, 1. лице једнине, презент.',
 '{"infinitive":"ићи","person":"1sg","tense":"presens"}',
 '{"answers":["идем"]}',
 'Глагол ићи припада 4. врсти, презент: идем, идеш, иде, идемо, идете, иду.',
 'seed', 'seed-conj-001', unixepoch()),

('conjugation', 3, 'verbs.perfekat',
 'Конјугација: писати, 3. лице једнине м. рода, перфекат.',
 '{"infinitive":"писати","person":"3sg.m","tense":"perfekat"}',
 '{"answers":["је писао"]}',
 'Перфекат: радни глаголски придев + презент глагола бити.',
 'seed', 'seed-conj-002', unixepoch()),

('conjugation', 4, 'verbs.perfekat',
 'Конјугација: бити, 2. лице множине ж. рода, перфекат.',
 '{"infinitive":"бити","person":"2pl.f","tense":"perfekat"}',
 '{"answers":["биле сте","сте биле"]}',
 'Радни придев бити: био/била/било, били/биле/била. За ж. род множине: биле сте.',
 'seed', 'seed-conj-003', unixepoch()),

('conjugation', 3, 'verbs.present',
 'Конјугација: моћи, 1. лице множине, презент.',
 '{"infinitive":"моћи","person":"1pl","tense":"presens"}',
 '{"answers":["можемо"]}',
 'Презент: могу, можеш, може, можемо, можете, могу.',
 'seed', 'seed-conj-004', unixepoch()),

('conjugation', 4, 'verbs.futur1',
 'Конјугација: видети, 3. лице једнине, футур I.',
 '{"infinitive":"видети","person":"3sg","tense":"futur1"}',
 '{"answers":["видеће","ће видети"]}',
 'Футур I: ће + инфинитив или енклитика + ће (видеће). У ћирилици се пише спојено када енклитика следи иза глагола.',
 'seed', 'seed-conj-005', unixepoch());

-- case: choose the correct form from options
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('case', 3, 'cases.loc-u',
 'Изабери: У ___ има много људи.',
 '{"stem":"У ___ има много људи.","options":["град","граду","граде"]}',
 '{"answers":["граду"]}',
 'У + локатив за место (где).',
 'seed', 'seed-case-001', unixepoch()),

('case', 3, 'cases.gen-bez',
 'Изабери: Без ___ не могу да отворим врата.',
 '{"stem":"Без ___ не могу да отворим врата.","options":["кључа","кључу","кључ"]}',
 '{"answers":["кључа"]}',
 'Предлог без увек тражи генитив.',
 'seed', 'seed-case-002', unixepoch()),

('case', 4, 'cases.gen-preko',
 'Изабери: Прешли смо преко ___ .',
 '{"stem":"Прешли смо преко ___ .","options":["мост","моста","мосту"]}',
 '{"answers":["моста"]}',
 'Предлог преко тражи генитив.',
 'seed', 'seed-case-003', unixepoch()),

('case', 4, 'cases.gen-zbog',
 'Изабери: Касним због ___ .',
 '{"stem":"Касним због ___ .","options":["саобраћаја","саобраћају","саобраћај"]}',
 '{"answers":["саобраћаја"]}',
 'Предлог због (узрок) тражи генитив.',
 'seed', 'seed-case-004', unixepoch()),

('case', 4, 'cases.dat-prema',
 'Изабери: Идемо према ___ .',
 '{"stem":"Идемо према ___ .","options":["центру","центар","центра"]}',
 '{"answers":["центру"]}',
 'Предлог према тражи датив.',
 'seed', 'seed-case-005', unixepoch());

-- aspect: pick perfective vs imperfective
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('aspect', 4, 'aspect.completion',
 'Изабери облик: Јуче сам ___ ту књигу до краја. (Я вчера прочитал эту книгу до конца.)',
 '{"ru":"Я вчера прочитал эту книгу до конца.","options":["читао","прочитао"]}',
 '{"answers":["прочитао"]}',
 'Перфективни глагол прочитати означава завршену радњу до краја.',
 'seed', 'seed-aspect-001', unixepoch()),

('aspect', 4, 'aspect.habit',
 'Изабери облик: Сваког јутра ___ новине. (Каждое утро читаю газеты.)',
 '{"ru":"Каждое утро читаю газеты.","options":["читам","прочитам"]}',
 '{"answers":["читам"]}',
 'Свакодневна (поновљена) радња тражи имперфективни вид.',
 'seed', 'seed-aspect-002', unixepoch()),

('aspect', 4, 'aspect.parallel',
 'Изабери облик: Док сам учио, мајка је ___ ручак. (Пока я учился, мама готовила обед.)',
 '{"ru":"Пока я учился, мама готовила обед.","options":["кувала","скувала"]}',
 '{"answers":["кувала"]}',
 'Паралелне радње у току тражеју имперфективни вид.',
 'seed', 'seed-aspect-003', unixepoch()),

('aspect', 5, 'aspect.future-completed',
 'Изабери облик: Сутра ћу ти ___ извештај. (Завтра напишу тебе отчёт.)',
 '{"ru":"Завтра напишу тебе отчёт.","options":["писати","написати"]}',
 '{"answers":["написати"]}',
 'У футуру I перфективни глагол означава да ће радња бити завршена.',
 'seed', 'seed-aspect-004', unixepoch()),

('aspect', 4, 'aspect.repeated-future',
 'Изабери облик: Сваке недеље ћу ти ___ . (Каждую неделю буду тебе звонить.)',
 '{"ru":"Каждую неделю буду тебе звонить.","options":["звати","позвати"]}',
 '{"answers":["звати"]}',
 'Понављана радња у будућности — имперфективни вид.',
 'seed', 'seed-aspect-005', unixepoch());

-- tr_ru_sr: translate Russian to Serbian (Cyrillic)
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('tr_ru_sr', 3, 'translation.daily',
 'Преведи на српски: «Я долго ждал тебя.»',
 '{"ru":"Я долго ждал тебя."}',
 '{"answers":["Дуго сам те чекао.","Дуго сам те чекала."],"must_contain":["дуго","чека"]}',
 'Дуго = долго; чекати кога (акузатив); перфекат са сам/си/је.',
 'seed', 'seed-tr-rs-001', unixepoch()),

('tr_ru_sr', 3, 'translation.daily',
 'Преведи на српски: «Мне нужно купить хлеб.»',
 '{"ru":"Мне нужно купить хлеб."}',
 '{"answers":["Морам да купим хлеб.","Треба да купим хлеб."],"must_contain":["купим","хлеб"]}',
 'Конструкција морам/треба + да + презент. У српском нема инфинитива иза модала по правилу.',
 'seed', 'seed-tr-rs-002', unixepoch()),

('tr_ru_sr', 3, 'translation.daily',
 'Преведи на српски: «Где ты живёшь?»',
 '{"ru":"Где ты живёшь?"}',
 '{"answers":["Где живиш?"]}',
 'Презент 2. лице једнине, без личне заменице (испуштена).',
 'seed', 'seed-tr-rs-003', unixepoch()),

('tr_ru_sr', 4, 'translation.daily',
 'Преведи на српски: «Извини за опоздание.»',
 '{"ru":"Извини за опоздание."}',
 '{"answers":["Извини што касним.","Извини због кашњења."],"must_contain":["извини"]}',
 'Две конструкције: што + презент (што касним) или због + генитив именице (због кашњења).',
 'seed', 'seed-tr-rs-004', unixepoch()),

('tr_ru_sr', 4, 'translation.daily',
 'Преведи на српски: «Я не понимаю, что ты говоришь.»',
 '{"ru":"Я не понимаю, что ты говоришь."}',
 '{"answers":["Не разумем шта говориш."],"must_contain":["разумем","шта","говориш"]}',
 'У српском се користи шта (што за зависну реченицу је регионално).',
 'seed', 'seed-tr-rs-005', unixepoch());

-- tr_sr_ru: translate Serbian to Russian (Russian Cyrillic)
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('tr_sr_ru', 3, 'translation.daily',
 'Преведи на руски: «Не могу да дођем сутра.»',
 '{"sr":"Не могу да дођем сутра."}',
 '{"answers":["Не могу прийти завтра.","Я не могу прийти завтра."],"must_contain":["не могу","завтра"]}',
 'Конструкција могу + да + презент = русское «могу + инф.».',
 'seed', 'seed-tr-rs-006', unixepoch()),

('tr_sr_ru', 3, 'translation.daily',
 'Преведи на руски: «Колико кошта ова кафа?»',
 '{"sr":"Колико кошта ова кафа?"}',
 '{"answers":["Сколько стоит этот кофе?","Сколько стоит эта кофе?"],"must_contain":["сколько стоит"]}',
 'Кафа (ж.р.) у српском, кофе (м.р.) у руском — обратите пажњу на род.',
 'seed', 'seed-tr-rs-007', unixepoch()),

('tr_sr_ru', 4, 'translation.daily',
 'Преведи на руски: «Зашто ниси јавио?»',
 '{"sr":"Зашто ниси јавио?"}',
 '{"answers":["Почему ты не сообщил?","Почему не сообщил?","Почему ты не дал знать?"],"must_contain":["почему"]}',
 'Јавити = сообщить/дать знать. Перфекат у питању.',
 'seed', 'seed-tr-rs-008', unixepoch()),

('tr_sr_ru', 3, 'translation.daily',
 'Преведи на руски: «Чекам аутобус већ пола сата.»',
 '{"sr":"Чекам аутобус већ пола сата."}',
 '{"answers":["Жду автобус уже полчаса.","Я жду автобус уже полчаса."],"must_contain":["жду","автобус","полчаса"]}',
 'У српском акузатив без предлога код чекати: чекам аутобус (не аутобуса).',
 'seed', 'seed-tr-rs-009', unixepoch()),

('tr_sr_ru', 4, 'translation.daily',
 'Преведи на руски: «Радујем се што ћемо се видети сутра.»',
 '{"sr":"Радујем се што ћемо се видети сутра."}',
 '{"answers":["Я рад, что мы увидимся завтра.","Радуюсь, что мы увидимся завтра."],"must_contain":["завтра","увидим"]}',
 'Радовати се + датив или + што-реченица. Футур I: ћемо се видети.',
 'seed', 'seed-tr-rs-010', unixepoch());

-- vocab: simple flashcards
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('vocab', 3, 'vocab.daily',
 'извињење',
 '{"front":"извињење","back":"извинение","pos":"noun.n"}',
 '{"answers":["извинение"]}',
 NULL, 'seed', 'seed-vocab-001', unixepoch()),

('vocab', 3, 'vocab.daily',
 'изненада',
 '{"front":"изненада","back":"внезапно, неожиданно","pos":"adv"}',
 '{"answers":["внезапно","неожиданно"]}',
 NULL, 'seed', 'seed-vocab-002', unixepoch()),

('vocab', 3, 'vocab.daily',
 'свакодневно',
 '{"front":"свакодневно","back":"ежедневно","pos":"adv"}',
 '{"answers":["ежедневно","каждодневно"]}',
 NULL, 'seed', 'seed-vocab-003', unixepoch()),

('vocab', 4, 'vocab.daily',
 'обавеза',
 '{"front":"обавеза","back":"обязанность","pos":"noun.f"}',
 '{"answers":["обязанность"]}',
 NULL, 'seed', 'seed-vocab-004', unixepoch()),

('vocab', 4, 'vocab.daily',
 'утицај',
 '{"front":"утицај","back":"влияние","pos":"noun.m"}',
 '{"answers":["влияние"]}',
 NULL, 'seed', 'seed-vocab-005', unixepoch());

-- speak: read the Cyrillic sentence aloud, Whisper compares
INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at) VALUES
('speak', 3, 'speak.greeting',
 'Прочитај наглас: «Добар дан, како сте?»',
 '{"target_sr":"Добар дан, како сте?"}',
 '{"answers":["добар дан како сте"]}',
 NULL, 'seed', 'seed-speak-001', unixepoch()),

('speak', 3, 'speak.request',
 'Прочитај наглас: «Желео бих да резервишем сто за двоје.»',
 '{"target_sr":"Желео бих да резервишем сто за двоје."}',
 '{"answers":["желео бих да резервишем сто за двоје","желела бих да резервишем сто за двоје"]}',
 NULL, 'seed', 'seed-speak-002', unixepoch()),

('speak', 3, 'speak.question',
 'Прочитај наглас: «Извините, не знам где је пошта.»',
 '{"target_sr":"Извините, не знам где је пошта."}',
 '{"answers":["извините не знам где је пошта"]}',
 NULL, 'seed', 'seed-speak-003', unixepoch()),

('speak', 3, 'speak.request',
 'Прочитај наглас: «Можете ли да говорите спорије, молим вас?»',
 '{"target_sr":"Можете ли да говорите спорије, молим вас?"}',
 '{"answers":["можете ли да говорите спорије молим вас"]}',
 NULL, 'seed', 'seed-speak-004', unixepoch()),

('speak', 4, 'speak.expression',
 'Прочитај наглас: «Радујем се што ћемо се видети сутра.»',
 '{"target_sr":"Радујем се што ћемо се видети сутра."}',
 '{"answers":["радујем се што ћемо се видети сутра"]}',
 NULL, 'seed', 'seed-speak-005', unixepoch());

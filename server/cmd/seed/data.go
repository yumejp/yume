package main

// Curated seed vocabulary. Real words, real sentences — every sentence is a
// natural example that demonstrates how the word is actually used (right
// collocation, typical register), and always contains the word's surface
// form so the review screen's sentence line and cloze blanking work.
// main() validates that invariant and fails loudly if an entry breaks it.

type seedCard struct {
	lemma   string
	kanji   string // "" for kana-only words
	reading string
	glosses []string
	// sentence shows the word in natural use; surface is the exact form the
	// word takes inside the sentence (inflected for verbs/adjectives).
	sentence string
	surface  string
	cloze    bool // seed as a cloze card (sentence-front, word blanked)
}

var seedCards = []seedCard{
	// --- Daily life nouns ---------------------------------------------------
	{"約束", "約束", "やくそく", []string{"promise", "appointment"}, "明日の約束を忘れないでください。", "約束", false},
	{"経験", "経験", "けいけん", []string{"experience"}, "留学の経験が今の仕事に役立っている。", "経験", false},
	{"準備", "準備", "じゅんび", []string{"preparation"}, "旅行の準備はもう終わりましたか。", "準備", false},
	{"予定", "予定", "よてい", []string{"plan", "schedule"}, "週末は特に予定がありません。", "予定", false},
	{"残業", "残業", "ざんぎょう", []string{"overtime work"}, "今日も残業で帰りが遅くなった。", "残業", false},
	{"遅刻", "遅刻", "ちこく", []string{"lateness", "being late"}, "電車が止まって会議に遅刻した。", "遅刻", false},
	{"連絡", "連絡", "れんらく", []string{"contact", "getting in touch"}, "駅に着いたらすぐ連絡します。", "連絡", false},
	{"相談", "相談", "そうだん", []string{"consultation", "asking for advice"}, "進路について先生に相談した。", "相談", false},
	{"紹介", "紹介", "しょうかい", []string{"introduction"}, "友達にいい歯医者を紹介してもらった。", "紹介", false},
	{"招待", "招待", "しょうたい", []string{"invitation"}, "大学時代の友人の結婚式に招待された。", "招待", false},
	{"挨拶", "挨拶", "あいさつ", []string{"greeting"}, "引っ越しの挨拶に隣の家へ行った。", "挨拶", false},
	{"都合", "都合", "つごう", []string{"convenience", "circumstances"}, "来週の都合のいい日を教えてください。", "都合", false},
	{"趣味", "趣味", "しゅみ", []string{"hobby"}, "趣味は写真を撮ることです。", "趣味", false},
	{"散歩", "散歩", "さんぽ", []string{"walk", "stroll"}, "毎朝、犬と川沿いを散歩している。", "散歩", false},
	{"思い出", "思い出", "おもいで", []string{"memory", "keepsake"}, "学生時代の思い出が詰まったアルバムを見つけた。", "思い出", false},
	{"気分", "気分", "きぶん", []string{"mood", "feeling"}, "少し散歩したら気分が良くなった。", "気分", false},
	{"安心", "安心", "あんしん", []string{"relief", "peace of mind"}, "家族が無事だと聞いて安心した。", "安心", false},
	{"心配", "心配", "しんぱい", []string{"worry", "concern"}, "連絡がないので心配している。", "心配", false},
	{"我慢", "我慢", "がまん", []string{"patience", "endurance"}, "歯の痛みを我慢して仕事を続けた。", "我慢", false},
	{"遠慮", "遠慮", "えんりょ", []string{"reserve", "holding back"}, "どうぞ遠慮しないで食べてください。", "遠慮", false},

	// --- Money / shopping ----------------------------------------------------
	{"値段", "値段", "ねだん", []string{"price"}, "この店は味のわりに値段が高い。", "値段", false},
	{"給料", "給料", "きゅうりょう", []string{"salary", "wages"}, "給料日は毎月二十五日です。", "給料", false},
	{"貯金", "貯金", "ちょきん", []string{"savings"}, "旅行のために毎月貯金している。", "貯金", false},
	{"財布", "財布", "さいふ", []string{"wallet"}, "財布を家に忘れて昼ご飯が買えなかった。", "財布", false},
	{"割引", "割引", "わりびき", []string{"discount"}, "学生証を見せると割引になります。", "割引", false},
	{"無料", "無料", "むりょう", []string{"free of charge"}, "二杯目のコーヒーは無料です。", "無料", false},
	{"会計", "会計", "かいけい", []string{"bill", "payment"}, "お会計は別々でお願いします。", "会計", false},
	{"注文", "注文", "ちゅうもん", []string{"order (e.g. food)"}, "注文した料理がなかなか来ない。", "注文", false},
	{"予約", "予約", "よやく", []string{"reservation", "booking"}, "人気の店なので予約が必要です。", "予約", false},
	{"家賃", "家賃", "やちん", []string{"rent"}, "東京は家賃が高すぎる。", "家賃", false},

	// --- Movement / travel ----------------------------------------------------
	{"出発", "出発", "しゅっぱつ", []string{"departure"}, "出発の三十分前に空港に着いた。", "出発", false},
	{"到着", "到着", "とうちゃく", []string{"arrival"}, "飛行機の到着が一時間遅れた。", "到着", false},
	{"旅行", "旅行", "りょこう", []string{"travel", "trip"}, "卒業したら友達と海外旅行に行きたい。", "旅行", false},
	{"観光", "観光", "かんこう", []string{"sightseeing"}, "観光より地元の食べ物に興味がある。", "観光", false},
	{"お土産", "お土産", "おみやげ", []string{"souvenir"}, "京都のお土産に八つ橋を買った。", "お土産", false},
	{"渋滞", "渋滞", "じゅうたい", []string{"traffic jam"}, "連休の高速道路は渋滞がひどい。", "渋滞", false},
	{"駐車場", "駐車場", "ちゅうしゃじょう", []string{"parking lot"}, "この辺りは駐車場が少なくて困る。", "駐車場", false},
	{"引っ越し", "引っ越し", "ひっこし", []string{"moving (house)"}, "来月、会社の近くに引っ越しする。", "引っ越し", false},
	{"連休", "連休", "れんきゅう", []string{"consecutive holidays"}, "連休はどこも人でいっぱいだ。", "連休", false},
	{"近所", "近所", "きんじょ", []string{"neighborhood"}, "近所においしいパン屋ができた。", "近所", false},
	{"隣", "隣", "となり", []string{"next to", "neighbor"}, "隣の部屋の音が気になって眠れない。", "隣", false},
	{"大家", "大家", "おおや", []string{"landlord"}, "水漏れのことを大家さんに相談した。", "大家", false},

	// --- Weather / seasons -----------------------------------------------------
	{"天気予報", "天気予報", "てんきよほう", []string{"weather forecast"}, "天気予報によると週末は晴れるそうだ。", "天気予報", false},
	{"梅雨", "梅雨", "つゆ", []string{"rainy season"}, "梅雨に入って毎日じめじめしている。", "梅雨", false},
	{"台風", "台風", "たいふう", []string{"typhoon"}, "台風で電車が全部止まった。", "台風", false},
	{"紅葉", "紅葉", "こうよう", []string{"autumn leaves"}, "京都へ紅葉を見に行く予定だ。", "紅葉", false},
	{"花見", "花見", "はなみ", []string{"cherry-blossom viewing"}, "公園で会社の仲間と花見をした。", "花見", false},

	// --- Health -------------------------------------------------------------
	{"風邪", "風邪", "かぜ", []string{"cold (illness)"}, "風邪をひいて声が出なくなった。", "風邪", false},
	{"熱", "熱", "ねつ", []string{"fever"}, "熱が三十八度もあって会社を休んだ。", "熱", false},
	{"薬", "薬", "くすり", []string{"medicine"}, "この薬は食後に飲んでください。", "薬", false},
	{"怪我", "怪我", "けが", []string{"injury"}, "階段で転んで怪我をした。", "怪我", false},
	{"病院", "病院", "びょういん", []string{"hospital"}, "朝から病院は混んでいた。", "病院", false},

	// --- Work / study ----------------------------------------------------------
	{"会議", "会議", "かいぎ", []string{"meeting", "conference"}, "会議は三時から第二会議室で行います。", "会議", false},
	{"資料", "資料", "しりょう", []string{"materials", "documents"}, "明日までに会議の資料を作らなければならない。", "資料", false},
	{"締め切り", "締め切り", "しめきり", []string{"deadline"}, "レポートの締め切りは金曜日だ。", "締め切り", false},
	{"試験", "試験", "しけん", []string{"exam"}, "試験の前の日は緊張して眠れない。", "試験", false},
	{"合格", "合格", "ごうかく", []string{"passing (an exam)"}, "一年勉強してやっと試験に合格した。", "合格", false},
	{"失敗", "失敗", "しっぱい", []string{"failure", "mistake"}, "失敗を恐れずに挑戦してほしい。", "失敗", false},
	{"成功", "成功", "せいこう", []string{"success"}, "実験は三回目でようやく成功した。", "成功", false},
	{"説明", "説明", "せつめい", []string{"explanation"}, "先生の説明はいつも分かりやすい。", "説明", false},
	{"質問", "質問", "しつもん", []string{"question"}, "質問がある人は手を挙げてください。", "質問", false},
	{"答え", "答え", "こたえ", []string{"answer"}, "この問題の答えがどうしても分からない。", "答え", false},
	{"意味", "意味", "いみ", []string{"meaning"}, "この漢字の意味を辞書で調べた。", "意味", false},
	{"言葉", "言葉", "ことば", []string{"word", "language"}, "国によって挨拶の言葉が違う。", "言葉", false},
	{"復習", "復習", "ふくしゅう", []string{"review (of lessons)"}, "授業の後で必ず復習するようにしている。", "復習", false},
	{"出席", "出席", "しゅっせき", []string{"attendance"}, "明日の会議に出席できますか。", "出席", false},
	{"欠席", "欠席", "けっせき", []string{"absence"}, "熱のため授業を欠席した。", "欠席", false},
	{"残念", "残念", "ざんねん", []string{"regrettable", "a pity"}, "楽しみにしていたのに中止になって残念だ。", "残念", false},

	// --- Society ---------------------------------------------------------------
	{"経済", "経済", "けいざい", []string{"economy"}, "円安が日本の経済に影響を与えている。", "経済", false},
	{"政治", "政治", "せいじ", []string{"politics"}, "若い人の政治への関心が低い。", "政治", false},
	{"環境", "環境", "かんきょう", []string{"environment"}, "環境のためにレジ袋を断っている。", "環境", false},
	{"社会", "社会", "しゃかい", []string{"society"}, "高齢化は日本の社会の大きな課題だ。", "社会", false},
	{"文化", "文化", "ぶんか", []string{"culture"}, "留学して初めて自分の国の文化に気づいた。", "文化", false},
	{"歴史", "歴史", "れきし", []string{"history"}, "この町には千年の歴史がある。", "歴史", false},
	{"自然", "自然", "しぜん", []string{"nature"}, "北海道は自然が豊かで食べ物もおいしい。", "自然", false},
	{"地震", "地震", "じしん", []string{"earthquake"}, "地震に備えて水と食料を用意している。", "地震", false},
	{"火事", "火事", "かじ", []string{"fire (incident)"}, "夜中に隣のビルで火事があった。", "火事", false},
	{"警察", "警察", "けいさつ", []string{"police"}, "事故を見てすぐ警察に電話した。", "警察", false},
	{"泥棒", "泥棒", "どろぼう", []string{"thief", "burglar"}, "泥棒に入られて窓が割られていた。", "泥棒", false},

	// --- Verbs (surface = inflected form in the sentence) ----------------------
	{"慣れる", "慣れる", "なれる", []string{"to get used to"}, "新しい生活にだんだん慣れてきた。", "慣れて", true},
	{"忘れる", "忘れる", "わすれる", []string{"to forget", "to leave behind"}, "傘を電車に忘れてしまった。", "忘れて", true},
	{"覚える", "覚える", "おぼえる", []string{"to memorize", "to learn"}, "一日に十個の単語を覚えるようにしている。", "覚える", false},
	{"続ける", "続ける", "つづける", []string{"to continue"}, "毎日少しずつ勉強を続けることが大切だ。", "続ける", true},
	{"決める", "決める", "きめる", []string{"to decide"}, "引っ越しの日をまだ決めていない。", "決めて", false},
	{"迎える", "迎える", "むかえる", []string{"to welcome", "to go to meet"}, "空港まで友達を迎えに行った。", "迎え", false},
	{"届く", "届く", "とどく", []string{"to be delivered", "to reach"}, "注文した本が今朝届いた。", "届いた", true},
	{"壊れる", "壊れる", "こわれる", []string{"to break (intr.)"}, "洗濯機が壊れて水が止まらない。", "壊れて", false},
	{"直す", "直す", "なおす", []string{"to fix", "to repair"}, "父は自分で自転車を直した。", "直した", false},
	{"探す", "探す", "さがす", []string{"to look for"}, "駅の近くで安いアパートを探している。", "探して", true},
	{"見つける", "見つける", "みつける", []string{"to find"}, "なくした鍵をやっと見つけた。", "見つけた", false},
	{"借りる", "借りる", "かりる", []string{"to borrow", "to rent"}, "図書館で本を三冊借りた。", "借りた", false},
	{"返す", "返す", "かえす", []string{"to return (something)"}, "借りたお金は必ず返します。", "返します", false},
	{"頼む", "頼む", "たのむ", []string{"to ask", "to request"}, "同僚に仕事の手伝いを頼んだ。", "頼んだ", true},
	{"断る", "断る", "ことわる", []string{"to refuse", "to decline"}, "上司の誘いを断る勇気がない。", "断る", true},
	{"混む", "混む", "こむ", []string{"to be crowded"}, "朝の電車はいつも混んでいる。", "混んで", false},
	{"並ぶ", "並ぶ", "ならぶ", []string{"to line up"}, "人気のラーメン屋に一時間も並んだ。", "並んだ", false},
	{"急ぐ", "急ぐ", "いそぐ", []string{"to hurry"}, "急がないと終電に間に合わないよ。", "急がない", false},
	{"間に合う", "間に合う", "まにあう", []string{"to be in time"}, "タクシーで行けばまだ間に合う。", "間に合う", true},
	{"足りる", "足りる", "たりる", []string{"to be enough"}, "時間が足りなくて最後の問題が解けなかった。", "足りなくて", false},
	{"増える", "増える", "ふえる", []string{"to increase"}, "最近、在宅勤務の人が増えている。", "増えて", true},
	{"減る", "減る", "へる", []string{"to decrease"}, "税金が引かれて手取りが減った。", "減った", false},
	{"変わる", "変わる", "かわる", []string{"to change (intr.)"}, "四月からバスの時刻表が変わります。", "変わります", false},
	{"比べる", "比べる", "くらべる", []string{"to compare"}, "値段を比べてから買うことにしている。", "比べて", false},
	{"選ぶ", "選ぶ", "えらぶ", []string{"to choose"}, "母へのプレゼントを選ぶのに二時間かかった。", "選ぶ", true},
	{"捨てる", "捨てる", "すてる", []string{"to throw away"}, "古い雑誌をまとめて捨てた。", "捨てた", false},
	{"拾う", "拾う", "ひろう", []string{"to pick up"}, "道で財布を拾って交番に届けた。", "拾って", false},
	{"渡る", "渡る", "わたる", []string{"to cross"}, "信号が青になってから道を渡りましょう。", "渡り", false},
	{"曲がる", "曲がる", "まがる", []string{"to turn", "to bend"}, "二つ目の角を右に曲がってください。", "曲がって", false},
	{"着く", "着く", "つく", []string{"to arrive"}, "駅に着いたら電話してね。", "着いたら", false},
	{"疲れる", "疲れる", "つかれる", []string{"to get tired"}, "一日中の立ち仕事で足が疲れた。", "疲れた", false},
	{"頑張る", "頑張る", "がんばる", []string{"to do one's best"}, "最後まで諦めずに頑張ります。", "頑張ります", true},
	{"諦める", "諦める", "あきらめる", []string{"to give up"}, "夢を諦めるのはまだ早い。", "諦める", true},

	// --- Adjectives -----------------------------------------------------------
	{"大丈夫", "大丈夫", "だいじょうぶ", []string{"all right", "okay"}, "熱は下がったから、もう大丈夫です。", "大丈夫", false},
	{"無理", "無理", "むり", []string{"impossible", "overdoing it"}, "無理をしないで早く寝てください。", "無理", false},
	{"大変", "大変", "たいへん", []string{"tough", "serious"}, "引っ越しは思ったより大変だった。", "大変", false},
	{"簡単", "簡単", "かんたん", []string{"easy", "simple"}, "このアプリは操作が簡単で使いやすい。", "簡単", false},
	{"複雑", "複雑", "ふくざつ", []string{"complicated"}, "駅の乗り換えが複雑で迷ってしまった。", "複雑", false},
	{"便利", "便利", "べんり", []string{"convenient"}, "駅前に住むと買い物が便利だ。", "便利", false},
	{"不便", "不便", "ふべん", []string{"inconvenient"}, "車がないと、この辺りは不便です。", "不便", false},
	{"危ない", "危ない", "あぶない", []string{"dangerous"}, "夜のこの道は暗くて危ない。", "危ない", false},
	{"安全", "安全", "あんぜん", []string{"safety", "safe"}, "安全のためにヘルメットをかぶってください。", "安全", false},
	{"静か", "静か", "しずか", []string{"quiet"}, "もう少し静かな席に変えてもらえますか。", "静か", false},
	{"賑やか", "賑やか", "にぎやか", []string{"lively", "bustling"}, "週末の商店街はとても賑やかだ。", "賑やか", false},
	{"寂しい", "寂しい", "さびしい", []string{"lonely"}, "一人暮らしは自由だけど、時々寂しい。", "寂しい", false},
	{"嬉しい", "嬉しい", "うれしい", []string{"happy", "glad"}, "合格の知らせを聞いて本当に嬉しかった。", "嬉しかった", false},
	{"悲しい", "悲しい", "かなしい", []string{"sad"}, "悲しい映画を見て泣いてしまった。", "悲しい", false},
	{"恥ずかしい", "恥ずかしい", "はずかしい", []string{"embarrassing", "ashamed"}, "人前で転んで恥ずかしかった。", "恥ずかしかった", false},
	{"怖い", "怖い", "こわい", []string{"scary", "frightening"}, "地震のニュースを見ると怖くなる。", "怖く", false},
	{"眠い", "眠い", "ねむい", []string{"sleepy"}, "昼ご飯の後はいつも眠くなる。", "眠く", false},
	{"楽しみ", "楽しみ", "たのしみ", []string{"looking forward to"}, "週末のキャンプが楽しみだ。", "楽しみ", false},

	// --- Kana-only / loanwords --------------------------------------------------
	{"きれい", "綺麗", "きれい", []string{"pretty", "clean"}, "週末に部屋をきれいに掃除した。", "きれい", false},
	{"コーヒー", "", "コーヒー", []string{"coffee"}, "朝はコーヒーを飲まないと目が覚めない。", "コーヒー", false},
	{"アルバイト", "", "アルバイト", []string{"part-time job"}, "週三日、コンビニでアルバイトをしている。", "アルバイト", false},
	{"パソコン", "", "パソコン", []string{"personal computer"}, "パソコンの調子が悪くて再起動した。", "パソコン", false},
	{"スマホ", "", "スマホ", []string{"smartphone"}, "寝る前にスマホを見るのをやめたい。", "スマホ", false},
	{"エアコン", "", "エアコン", []string{"air conditioner"}, "エアコンをつけたまま寝てしまった。", "エアコン", false},
}

// Real common words for word_status rows (they show up in the vocab
// explorer's table, so 「status語37」-style fake lemmas looked broken).
// Statuses are assigned in main(): mostly known, some learning, a few
// ignored (particles/grammar words a user would typically ignore).
var statusWords = []string{
	"水", "火", "山", "川", "空", "海", "月", "日", "年", "時間",
	"今", "朝", "昼", "夜", "晩", "春", "夏", "秋", "冬", "毎日",
	"東", "西", "南", "北", "右", "左", "上", "下", "中", "外",
	"前", "後ろ", "家", "部屋", "学校", "先生", "学生", "友達", "父", "母",
	"兄", "弟", "姉", "妹", "子供", "男", "女", "人", "名前", "電話",
	"電車", "駅", "道", "車", "自転車", "飛行機", "切符", "地図", "店", "銀行",
	"郵便局", "図書館", "公園", "映画", "音楽", "歌", "絵", "本", "新聞", "雑誌",
	"辞書", "紙", "鉛筆", "机", "椅子", "窓", "鍵", "傘", "帽子", "靴",
	"服", "鞄", "時計", "眼鏡", "犬", "猫", "鳥", "魚", "牛", "馬",
	"肉", "野菜", "果物", "卵", "米", "パン", "茶", "牛乳", "酒", "料理",
	"朝ご飯", "昼ご飯", "晩ご飯", "塩", "砂糖", "醤油", "箸", "皿", "風呂", "玄関",
	"庭", "池", "花", "木", "草", "石", "雪", "雨", "風", "雲",
	"星", "体", "頭", "顔", "目", "耳", "鼻", "口", "手", "足",
	"声", "病気", "元気", "力", "心", "気持ち", "問題", "宿題", "授業", "教室",
	"黒板", "漢字", "平仮名", "片仮名", "作文", "留学生", "外国", "外国人", "国", "町",
	"村", "世界", "地下鉄", "バス", "タクシー", "ホテル", "レストラン", "デパート", "スーパー", "コンビニ",
	"エレベーター", "プール", "テレビ", "ラジオ", "カメラ", "ドア", "テーブル", "ベッド", "シャワー", "ニュース",
	"する", "なる", "行く", "来る", "帰る", "食べる", "飲む", "見る", "聞く", "読む",
	"書く", "話す", "買う", "売る", "立つ", "座る", "歩く", "走る", "泳ぐ", "飛ぶ",
	"寝る", "起きる", "働く", "休む", "遊ぶ", "待つ", "会う", "持つ", "使う", "作る",
	"開ける", "閉める", "入る", "出る", "乗る", "降りる", "始まる", "終わる", "教える", "習う",
	"大きい", "小さい", "高い", "安い", "新しい", "古い", "良い", "悪い", "暑い", "寒い",
	"熱い", "冷たい", "難しい", "易しい", "面白い", "つまらない", "早い", "遅い", "近い", "遠い",
	"強い", "弱い", "長い", "短い", "明るい", "暗い", "重い", "軽い", "白い", "黒い",
}

// ignoredWords get status 3 — the kind of grammar glue a real user marks
// "ignore" so it stops counting against coverage.
var ignoredWords = []string{
	"は", "が", "を", "に", "で", "と", "も", "へ", "や", "の",
	"です", "ます", "から", "まで",
}

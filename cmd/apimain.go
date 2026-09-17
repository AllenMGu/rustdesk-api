package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http"
	"github.com/lejianwen/rustdesk-api/v2/lib/cache"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
	"github.com/lejianwen/rustdesk-api/v2/lib/orm"
	"github.com/lejianwen/rustdesk-api/v2/lib/upload"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"
)

// 266: AutoMigrate 应用 model.Peer.Id 的 uniqueIndex（idx_peers_id_unique）。
// 既有部署在版本升级时自动执行 Migrate()，为 peers.id 创建唯一索引；
// 索引必须真实建成（ensurePeersIdUniqueIndex 校验）才记录 266，
// 失败（如存在重复 id 行）时版本保持 265、服务仍可启动（降级为先
// 后到达），清理重复数据后下次启动自动重试，不会留下假迁移成功。
const DatabaseVersion = 266

// @title 管理系统API
// @version 1.0
// @description 接口
// @basePath /api
// @securityDefinitions.apikey token
// @in header
// @name api-token
// @securitydefinitions.apikey BearerAuth
// @in header
// @name Authorization

var rootCmd = &cobra.Command{
	Use:   "apimain",
	Short: "RUSTDESK API SERVER",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		InitGlobal()
	},
	Run: func(cmd *cobra.Command, args []string) {
		global.Logger.Info("API SERVER START")
		http.ApiInit()
	},
}

var resetPwdCmd = &cobra.Command{
	Use:     "reset-admin-pwd [pwd]",
	Example: "reset-admin-pwd 123456",
	Short:   "Reset Admin Password",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pwd := args[0]
		admin := service.AllService.UserService.InfoById(1)
		if admin.Id == 0 {
			global.Logger.Warn("user not found! ")
			return
		}
		err := service.AllService.UserService.UpdatePassword(admin, pwd)
		if err != nil {
			global.Logger.Error("reset password fail! ", err)
			return
		}
		global.Logger.Info("reset password success! ")
	},
}
var resetUserPwdCmd = &cobra.Command{
	Use:     "reset-pwd [userId] [pwd]",
	Example: "reset-pwd 2 123456",
	Short:   "Reset User Password",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		userId := args[0]
		pwd := args[1]
		uid, err := strconv.Atoi(userId)
		if err != nil {
			global.Logger.Warn("userId must be int!")
			return
		}
		if uid <= 0 {
			global.Logger.Warn("userId must be greater than 0! ")
			return
		}
		u := service.AllService.UserService.InfoById(uint(uid))
		if u.Id == 0 {
			global.Logger.Warn("user not found! ")
			return
		}
		err = service.AllService.UserService.UpdatePassword(u, pwd)
		if err != nil {
			global.Logger.Warn("reset password fail! ", err)
			return
		}
		global.Logger.Info("reset password success!")
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&global.ConfigPath, "config", "c", "./conf/config.yaml", "choose config file")
	rootCmd.AddCommand(resetPwdCmd, resetUserPwdCmd)
}
func main() {
	if err := rootCmd.Execute(); err != nil {
		global.Logger.Error(err)
		os.Exit(1)
	}
}

func InitGlobal() {
	//配置解析
	global.Viper = config.Init(&global.Config, global.ConfigPath)

	//日志
	global.Logger = logger.New(&logger.Config{
		Path:         global.Config.Logger.Path,
		Level:        global.Config.Logger.Level,
		ReportCaller: global.Config.Logger.ReportCaller,
	})

	global.InitI18n()

	//redis
	global.Redis = redis.NewClient(&redis.Options{
		Addr:     global.Config.Redis.Addr,
		Password: global.Config.Redis.Password,
		DB:       global.Config.Redis.Db,
	})

	//cache
	if global.Config.Cache.Type == cache.TypeFile {
		fc := cache.NewFileCache()
		fc.SetDir(global.Config.Cache.FileDir)
		global.Cache = fc
	} else if global.Config.Cache.Type == cache.TypeRedis {
		global.Cache = cache.NewRedis(&redis.Options{
			Addr:     global.Config.Cache.RedisAddr,
			Password: global.Config.Cache.RedisPwd,
			DB:       global.Config.Cache.RedisDb,
		})
	}
	//gorm
	if global.Config.Gorm.Type == config.TypeMysql {

		dsn := fmt.Sprintf("%s:%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s",
			global.Config.Mysql.Username,
			global.Config.Mysql.Password,
			global.Config.Mysql.Addr,
			global.Config.Mysql.Dbname,
			global.Config.Mysql.Tls,
		)

		global.DB = orm.NewMysql(&orm.MysqlConfig{
			Dsn:          dsn,
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	} else if global.Config.Gorm.Type == config.TypePostgresql {
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			global.Config.Postgresql.Host,
			global.Config.Postgresql.Port,
			global.Config.Postgresql.User,
			global.Config.Postgresql.Password,
			global.Config.Postgresql.Dbname,
			global.Config.Postgresql.Sslmode,
			global.Config.Postgresql.TimeZone,
		)
		global.DB = orm.NewPostgresql(&orm.PostgresqlConfig{
			Dsn:          dsn,
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	} else {
		//sqlite
		global.DB = orm.NewSqlite(&orm.SqliteConfig{
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	}

	//validator
	global.ApiInitValidator()

	//oss
	global.Oss = &upload.Oss{
		AccessKeyId:     global.Config.Oss.AccessKeyId,
		AccessKeySecret: global.Config.Oss.AccessKeySecret,
		Host:            global.Config.Oss.Host,
		CallbackUrl:     global.Config.Oss.CallbackUrl,
		ExpireTime:      global.Config.Oss.ExpireTime,
		MaxByte:         global.Config.Oss.MaxByte,
	}

	//jwt
	//fmt.Println(global.Config.Jwt.PrivateKey)
	global.Jwt = jwt.NewJwt(global.Config.Jwt.Key, global.Config.Jwt.ExpireDuration)
	//locker
	global.Lock = lock.NewLocal()

	//service
	service.New(&global.Config, global.DB, global.Logger, global.Jwt, global.Lock)

	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{
		CaptchaThreshold:     global.Config.App.CaptchaThreshold,
		BanThreshold:         global.Config.App.BanThreshold,
		AccountFailThreshold: global.Config.App.AccountFailThreshold,
		AccountBanDuration:   global.Config.App.AccountBanDuration,
		AttemptsWindow:       10 * time.Minute,
		BanDuration:          30 * time.Minute,
	})
	global.LoginLimiter.RegisterProvider(utils.B64StringCaptchaProvider{})

	// 匿名遥测端点（sysinfo/audit）限流器
	global.RateLimiter = utils.NewRateLimiter()
	DatabaseAutoUpdate()
}

func DatabaseAutoUpdate() {
	version := DatabaseVersion

	db := global.DB

	if global.Config.Gorm.Type == config.TypeMysql {
		//检查存不存在数据库，不存在则创建
		dbName := db.Migrator().CurrentDatabase()
		if dbName == "" {
			dbName = global.Config.Mysql.Dbname
			// 移除 DSN 中的数据库名称，以便初始连接时不指定数据库
			dsnWithoutDB := fmt.Sprintf("%s:%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				global.Config.Mysql.Username,
				global.Config.Mysql.Password,
				global.Config.Mysql.Addr,
				"",
			)

			//新链接
			dbWithoutDB := orm.NewMysql(&orm.MysqlConfig{
				Dsn: dsnWithoutDB,
			}, global.Logger)
			// 获取底层的 *sql.DB 对象，并确保在程序退出时关闭连接
			sqlDBWithoutDB, err := dbWithoutDB.DB()
			if err != nil {
				global.Logger.Errorf("获取底层 *sql.DB 对象失败: %v", err)
				return
			}
			defer func() {
				if err := sqlDBWithoutDB.Close(); err != nil {
					global.Logger.Errorf("关闭连接失败: %v", err)
				}
			}()

			err = dbWithoutDB.Exec("CREATE DATABASE IF NOT EXISTS " + dbName + " DEFAULT CHARSET utf8mb4").Error
			if err != nil {
				global.Logger.Error(err)
				return
			}
		}
	}

	if !db.Migrator().HasTable(&model.Version{}) {
		Migrate(uint(version))
	} else {
		//查找最后一个version
		var v model.Version
		db.Last(&v)
		if v.Version < uint(version) {
			Migrate(uint(version))
		}

		// 245迁移
		if v.Version < 245 {
			//oauths 表的 oauth_type 字段设置为 op同样的值
			db.Exec("update oauths set oauth_type = op")
			db.Exec("update oauths set issuer = 'https://accounts.google.com' where op = 'google'")
			db.Exec("update user_thirds set oauth_type = third_type, op = third_type")
			//通过email迁移旧的google授权
			uts := make([]model.UserThird, 0)
			db.Where("oauth_type = ?", "google").Find(&uts)
			for _, ut := range uts {
				if ut.UserId > 0 {
					db.Model(&model.User{}).Where("id = ?", ut.UserId).Update("email", ut.OpenId)
				}
			}
		}
		if v.Version < 246 {
			db.Exec("update oauths set issuer = 'https://accounts.google.com' where op = 'google' and issuer is null")
		}
	}

}
func Migrate(version uint) {
	global.Logger.Info("Migrating....", version)
	err := global.DB.AutoMigrate(
		&model.Version{},
		&model.User{},
		&model.UserToken{},
		&model.Tag{},
		&model.AddressBook{},
		&model.Peer{},
		&model.Group{},
		&model.UserThird{},
		&model.Oauth{},
		&model.LoginLog{},
		&model.ShareRecord{},
		&model.AuditConn{},
		&model.AuditFile{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
		&model.ServerCmd{},
		&model.DeviceGroup{},
	)
	if err != nil {
		global.Logger.Error("migrate err :=>", err)
		return
	}
	// v266 守卫：peers.id 唯一索引必须真实建成才允许记录新版本号。
	// 失败时版本保持旧值（如 265），下次启动自动重试迁移；
	// 否则"建索引失败 → 却永久标记为 266"会造成假迁移成功，
	// 管理员清理重复数据后重启也不会再建索引。
	if version >= DatabaseVersion && !ensurePeersIdUniqueIndex() {
		return
	}
	if err := global.DB.Create(&model.Version{Version: version}).Error; err != nil {
		global.Logger.Error("save database version failed: ", err)
		return
	}
	//如果是初次则创建一个默认用户
	var vc int64
	global.DB.Model(&model.Version{}).Count(&vc)
	if vc == 1 {
		localizer := global.Localizer("")
		defaultGroup, _ := localizer.LocalizeMessage(&i18n.Message{
			ID: "DefaultGroup",
		})
		group := &model.Group{
			Name: defaultGroup,
			Type: model.GroupTypeDefault,
		}
		service.AllService.GroupService.Create(group)

		shareGroup, _ := localizer.LocalizeMessage(&i18n.Message{
			ID: "ShareGroup",
		})
		groupShare := &model.Group{
			Name: shareGroup,
			Type: model.GroupTypeShare,
		}
		service.AllService.GroupService.Create(groupShare)
		//是true
		is_admin := true
		admin := &model.User{
			Username: "admin",
			Nickname: "Admin",
			Status:   model.COMMON_STATUS_ENABLE,
			IsAdmin:  &is_admin,
			GroupId:  1,
		}

		// 生成随机密码
		pwd := utils.RandomString(8)
		global.Logger.Info("Admin Password Is: ", pwd)
		var err error
		admin.Password, err = utils.EncryptPassword(pwd)
		if err != nil {
			global.Logger.Fatalf("failed to generate admin password: %v", err)
		}
		global.DB.Create(admin)
	}

}

// ensurePeersIdUniqueIndex 校验 v266 迁移是否真实完成：
//  1. peers.id 无重复行（重复行会阻止唯一索引创建）；
//  2. 索引 idx_peers_id_unique 真实存在，且必须同时满足：
//     属于 peers 表（当前 schema）+ 是 UNIQUE 索引 + 索引列恰好是
//     单列 id（AutoMigrate 对"同名索引已存在"会静默跳过，必须显式
//     验证表、唯一性与列——只查索引名不足以证明"唯一的是 peers.id"，
//     同名但建在别的列/复合列上的唯一索引不得记为迁移成功）。
//
// 任一检查失败返回 false：调用方不记录新版本号，服务可继续启动
// （降级为先到先得 + 日志），下次启动自动重试迁移。
func ensurePeersIdUniqueIndex() bool {
	// 1) 重复 id 检查（标准 SQL，sqlite/mysql/postgresql 通用）
	var dupGroups int64
	if err := global.DB.Raw("SELECT COUNT(*) FROM (SELECT id FROM peers GROUP BY id HAVING COUNT(*) > 1)").Scan(&dupGroups).Error; err != nil {
		global.Logger.Error("v266 guard: duplicate-id check failed: ", err)
		return false
	}
	if dupGroups > 0 {
		global.Logger.Errorf("v266 guard: peers.id 存在 %d 组重复行，唯一索引无法创建；请清理重复设备后重启（数据库版本保持旧值，下次启动自动重试）", dupGroups)
		return false
	}

	// 2) 按方言验证唯一索引存在且唯一
	var existsUnique bool
	var checkErr error
	switch global.Config.Gorm.Type {
	case config.TypeMysql:
		// information_schema.statistics 每个索引列一行：
		// 要求该索引在 peers 表上恰好有 1 列（排除复合列），
		// 且这一列是 id、索引为唯一（non_unique = 0）
		var totalCols, idCols int64
		checkErr = global.DB.Raw(
			"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'peers' AND index_name = 'idx_peers_id_unique'",
		).Scan(&totalCols).Error
		if checkErr == nil {
			checkErr = global.DB.Raw(
				"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'peers' AND index_name = 'idx_peers_id_unique' AND non_unique = 0 AND column_name = 'id'",
			).Scan(&idCols).Error
		}
		existsUnique = checkErr == nil && totalCols == 1 && idCols == 1
	case config.TypePostgresql:
		// 索引必须属于当前 schema 下的 peers 表、是 UNIQUE 索引、
		// 恰好 1 个索引列（indkey），且该列是 id
		var n, colN int64
		checkErr = global.DB.Raw(
			"SELECT COUNT(*) FROM pg_index i " +
				"JOIN pg_class ic ON ic.oid = i.indexrelid " +
				"JOIN pg_class tc ON tc.oid = i.indrelid " +
				"JOIN pg_namespace n ON n.oid = ic.relnamespace " +
				"WHERE ic.relname = 'idx_peers_id_unique' AND n.nspname = current_schema() " +
				"AND tc.relname = 'peers' AND i.indisunique AND array_length(i.indkey, 1) = 1",
		).Scan(&n).Error
		if checkErr == nil {
			checkErr = global.DB.Raw(
				"SELECT COUNT(*) FROM pg_index i " +
					"JOIN pg_class ic ON ic.oid = i.indexrelid " +
					"JOIN pg_class tc ON tc.oid = i.indrelid " +
					"JOIN pg_namespace n ON n.oid = ic.relnamespace " +
					"JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey) " +
					"WHERE ic.relname = 'idx_peers_id_unique' AND n.nspname = current_schema() " +
					"AND tc.relname = 'peers' AND i.indisunique AND a.attname = 'id'",
			).Scan(&colN).Error
		}
		existsUnique = checkErr == nil && n == 1 && colN == 1
	default: // sqlite
		// sqlite_master 已限定 tbl_name='peers' 且索引名为 idx_peers_id_unique；
		// 还需 UNIQUE INDEX 且索引列恰好为单列 id（防"同名但建在别的列/
		// 复合列"的唯一索引被误判为迁移成功）
		var sqlText string
		checkErr = global.DB.Raw(
			"SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'peers' AND name = 'idx_peers_id_unique'",
		).Scan(&sqlText).Error
		existsUnique = checkErr == nil &&
			strings.Contains(strings.ToUpper(sqlText), "UNIQUE INDEX") &&
			sqliteIndexColumn(sqlText) == "id"
	}
	if !existsUnique {
		global.Logger.Errorf("v266 guard: 唯一索引 idx_peers_id_unique 未就位（err=%v，要求：peers 表 + UNIQUE + 单列 id）；数据库版本保持旧值，下次启动自动重试", checkErr)
		return false
	}
	return true
}

// sqliteIndexColumnsRe 从索引 DDL 中提取 "ON <表名> (<列列表>)"：
// 表名允许双引号/反引号/方括号/裸名四种写法；列列表取第一个括号组。
var sqliteIndexColumnsRe = regexp.MustCompile(
	`(?is)\bON\s+(?:"([^"]+)"|` + "`([^`]*)`" + `|\[([^\]]*)\]|([A-Za-z_][A-Za-z0-9_.$]*))\s*\(([^)]*)\)`)

// sqliteIndexColumn 返回索引 DDL 的索引列；仅当索引恰好是单列时返回列名
// （剥掉引号/反引号/方括号），否则返回空串（复合列、表达式列、解析
// 失败一律按"不满足"处理——守卫宁严勿松）。
func sqliteIndexColumn(indexDDL string) string {
	m := sqliteIndexColumnsRe.FindStringSubmatch(indexDDL)
	if m == nil {
		return ""
	}
	table := firstNonEmpty(m[1], m[2], m[3], m[4])
	if !strings.EqualFold(table, "peers") {
		return ""
	}
	cols := strings.Split(m[5], ",")
	if len(cols) != 1 {
		return ""
	}
	col := strings.TrimSpace(cols[0])
	for {
		col = strings.TrimSpace(col)
		if len(col) >= 2 &&
			((col[0] == '"' && col[len(col)-1] == '"') ||
				(col[0] == '`' && col[len(col)-1] == '`') ||
				(col[0] == '[' && col[len(col)-1] == ']')) {
			col = col[1 : len(col)-1]
			continue
		}
		return strings.TrimSpace(col)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/automuteus/automuteus/v8/bot"
	"github.com/automuteus/automuteus/v8/bot/command"
	"github.com/automuteus/automuteus/v8/bot/tokenprovider"
	"github.com/automuteus/automuteus/v8/internal/server"
	"github.com/automuteus/automuteus/v8/pkg/capture"
	"github.com/automuteus/automuteus/v8/pkg/locale"
	storage2 "github.com/automuteus/automuteus/v8/pkg/storage"
	"github.com/automuteus/automuteus/v8/storage"
	"github.com/bwmarrin/discordgo"
)

var (
	version = "v8.1.0"
	commit  = "none"
	date    = "unknown"
)

//go:embed storage/postgres.sql
var postgresFileContents string

const (
	DefaultURL                   = "http://localhost:8123"
	DefaultMaxRequests5Sec int64 = 7
)

type registeredCommand struct {
	GuildID            string
	ApplicationCommand *discordgo.ApplicationCommand
}

// true: Discordへ登録する / false: 登録せず、既存登録があれば削除する。
// 表示・操作方法を変えないため、現在利用しているコマンドだけを明示します。
var EnabledSlashCommands = map[string]bool{
	"help":     true,
	"start":    true,
	"refresh":  false,
	"pause":    false,
	"stop":     true,
	"link":     true,
	"unlink":   true,
	"settings": true,
	"privacy":  false,
	"info":     false,
	"map":      false,
	"stats":    false,
	"premium":  false,
	"debug":    false,
	"download": false,
}

func isSlashCommandEnabled(name string) bool {
	enabled, ok := EnabledSlashCommands[name]
	return ok && enabled
}

func main() {
	// connect code generation用の乱数を初期化します。
	rand.Seed(time.Now().Unix())

	os.Exit(runMain(discordMainWrapper))
}

func runMain(runDiscord func() error) int {
	if err := runDiscord(); err != nil {
		log.Println("Program exited with the following error:")
		log.Println(err)
		return 1
	}

	return 0
}

func discordMainWrapper() error {
	isOfficial := os.Getenv("AUTOMUTEUS_OFFICIAL") != ""
	discordToken := os.Getenv("DISCORD_BOT_TOKEN")
	if discordToken == "" {
		return errors.New("no DISCORD_BOT_TOKEN provided")
	}

	logPath := os.Getenv("LOG_PATH")
	if logPath == "" {
		logPath = "./"
	}
	if os.Getenv("DISABLE_LOG_FILE") == "" {
		if err := os.MkdirAll(logPath, 0o755); err != nil {
			return fmt.Errorf("create log directory: %w", err)
		}
		file, err := os.OpenFile(path.Join(logPath, "logs.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				log.Printf("close log file: %v", err)
			}
		}()
		log.SetOutput(io.MultiWriter(os.Stdout, file))
	}

	// コンテナ内のログ時刻をJSTへ固定します。
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)
	time.Local = jst
	log.Println("Init: time.Local forced to Asia/Tokyo (UTC+9)")

	emojiGuildID := os.Getenv("EMOJI_GUILD_ID")
	log.Println(version + "-" + commit)

	numShards, err := strconv.Atoi(os.Getenv("NUM_SHARDS"))
	if err != nil {
		log.Println("No NUM_SHARDS specified; defaulting to 1")
		numShards = 1
	}
	if os.Getenv("SHARD_ID") != "" {
		return errors.New("SHARD_ID is no longer supported! Please use SHARDS instead")
	}

	var shardList shards
	shardsStr := os.Getenv("SHARDS")
	if shardsStr == "" {
		log.Println("No SHARDS specified, defaulting to 0")
		shardList = defaultShard()
	} else {
		shardList, err = parseShards(shardsStr, numShards)
		if err != nil {
			return err
		}
	}

	url := os.Getenv("HOST")
	if url == "" {
		log.Printf("[Info] No valid HOST provided. Defaulting to %s\n", DefaultURL)
		url = DefaultURL
	}

	var redisClient bot.RedisInterface
	var storageInterface storage.StorageInterface
	redisAddr := os.Getenv("REDIS_ADDR")
	redisPassword := os.Getenv("REDIS_PASS")
	if redisAddr == "" {
		return errors.New("no REDIS_ADDR specified; exiting")
	}
	if err := redisClient.Init(storage.RedisParameters{
		Addr:     redisAddr,
		Username: "",
		Password: redisPassword,
	}); err != nil {
		return fmt.Errorf("redis init failed: %w", err)
	}
	if err := storageInterface.Init(storage.RedisParameters{
		Addr:     redisAddr,
		Username: "",
		Password: redisPassword,
	}); err != nil {
		return fmt.Errorf("redis storage init failed: %w", err)
	}

	locale.InitLang(os.Getenv("LOCALE_PATH"), os.Getenv("BOT_LANG"))

	psql := storage2.PsqlInterface{}
	pAddr := os.Getenv("POSTGRES_ADDR")
	if pAddr == "" {
		return errors.New("no POSTGRES_ADDR specified; exiting")
	}
	pUser := os.Getenv("POSTGRES_USER")
	if pUser == "" {
		return errors.New("no POSTGRES_USER specified; exiting")
	}
	pPass := os.Getenv("POSTGRES_PASS")
	if pPass == "" {
		return errors.New("no POSTGRES_PASS specified; exiting")
	}
	if err := psql.Init(storage2.ConstructPsqlConnectURL(pAddr, pUser, pPass)); err != nil {
		return fmt.Errorf("postgres init failed: %w", err)
	}

	// DB準備が終わる前にDiscordコマンドを受け付けないよう、同期実行します。
	if !isOfficial {
		if err := psql.ExecFromString(postgresFileContents); err != nil {
			return fmt.Errorf("execute postgres.sql: %w", err)
		}
	}

	log.Println("Bot is starting. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sc)

	go server.StartHealthCheckServer("8080")

	topGGToken := os.Getenv("TOP_GG_TOKEN")
	taskTimeoutms := capture.DefaultCaptureBotTimeout
	if num, parseErr := strconv.ParseInt(os.Getenv("ACK_TIMEOUT_MS"), 10, 64); parseErr == nil {
		log.Printf("Read from env; using ACK_TIMEOUT_MS=%d\n", num)
		taskTimeoutms = time.Millisecond * time.Duration(num)
	}

	maxReq := DefaultMaxRequests5Sec
	if num, parseErr := strconv.ParseInt(os.Getenv("MAX_REQ_5_SEC"), 10, 64); parseErr == nil {
		maxReq = num
	}

	tokenProvider := tokenprovider.NewTokenProvider(nil, nil, taskTimeoutms, maxReq)
	var extraTokens []string
	extraTokenStr := strings.ReplaceAll(os.Getenv("WORKER_BOT_TOKENS"), " ", "")
	if extraTokenStr != "" {
		extraTokens = strings.Split(extraTokenStr, ",")
	}

	bots := make([]*bot.Bot, len(shardList))
	for i, shard := range shardList {
		bots[i] = bot.MakeAndStartBot(
			version,
			commit,
			discordToken,
			topGGToken,
			url,
			emojiGuildID,
			numShards,
			int(shard),
			&redisClient,
			&storageInterface,
			&psql,
			logPath,
		)
		if bots[i] == nil {
			for _, startedBot := range bots[:i] {
				if startedBot != nil {
					startedBot.Close()
				}
			}
			return fmt.Errorf("bot %d failed to initialize; check the Discord bot token and Discord connection", shard)
		}
	}

	bots[0].InitTokenProvider(tokenProvider)
	for i := range shardList {
		bots[i].TokenProvider = tokenProvider
	}
	tokenProvider.PopulateAndStartSessions(extraTokens)
	defer func() {
		for _, runningBot := range bots {
			if runningBot != nil {
				runningBot.Close()
			}
		}
		tokenProvider.Close()
	}()

	go bots[0].StartMetricsServer(os.Getenv("SCW_NODE_ID"))
	go bots[0].StartAPIServer("5000")

	if strings.TrimSpace(os.Getenv("API_SERVER_URL")) == "" {
		log.Println("[WARN] API_SERVER_URL is empty. /start will keep the host/code copy display, but the Capture launch link will be hidden.")
	}

	// empty string entry = global commands
	slashCommandGuildIDs := []string{""}
	slashCommandGuildIDStr := strings.ReplaceAll(os.Getenv("SLASH_COMMAND_GUILD_IDS"), " ", "")
	if slashCommandGuildIDStr != "" {
		slashCommandGuildIDs = strings.Split(slashCommandGuildIDStr, ",")
	}

	var registeredCommands []registeredCommand
	if !isOfficial || shardList.isPrimaryShard() {
		for _, guild := range slashCommandGuildIDs {
			existing, fetchErr := bots[0].PrimarySession.ApplicationCommands(
				bots[0].PrimarySession.State.User.ID,
				guild,
			)
			if fetchErr != nil {
				log.Printf("Cannot fetch existing commands for guild %q: %v", guild, fetchErr)
			} else {
				for _, existingCommand := range existing {
					if isSlashCommandEnabled(existingCommand.Name) {
						continue
					}
					if deleteErr := bots[0].PrimarySession.ApplicationCommandDelete(
						existingCommand.ApplicationID,
						guild,
						existingCommand.ID,
					); deleteErr != nil {
						log.Printf("Failed to delete disabled command %s in guild %q: %v", existingCommand.Name, guild, deleteErr)
					} else if guild == "" {
						log.Printf("Deleted disabled command %s GLOBALLY\n", existingCommand.Name)
					} else {
						log.Printf("Deleted disabled command %s in guild %s\n", existingCommand.Name, guild)
					}
				}
			}

			for _, applicationCommand := range command.All {
				if !isSlashCommandEnabled(applicationCommand.Name) {
					if guild == "" {
						log.Printf("Skip disabled command %s GLOBALLY\n", applicationCommand.Name)
					} else {
						log.Printf("Skip disabled command %s in guild %s\n", applicationCommand.Name, guild)
					}
					continue
				}

				if guild == "" {
					log.Printf("Registering command %s GLOBALLY\n", applicationCommand.Name)
				} else {
					log.Printf("Registering command %s in guild %s\n", applicationCommand.Name, guild)
				}

				createdCommand, createErr := bots[0].PrimarySession.ApplicationCommandCreate(
					bots[0].PrimarySession.State.User.ID,
					guild,
					applicationCommand,
				)
				if createErr != nil {
					return fmt.Errorf("create command %s for guild %q: %w", applicationCommand.Name, guild, createErr)
				}
				registeredCommands = append(registeredCommands, registeredCommand{
					GuildID:            guild,
					ApplicationCommand: createdCommand,
				})
			}
		}
		log.Println("Finishing registering all commands!")
	}

	// Redis/Postgres/Discord/コマンド登録が完了してからreadyにします。
	server.GlobalReady = true
	log.Println("Bot startup checks completed; health endpoint is ready")

	<-sc
	server.GlobalReady = false
	log.Println("Received shutdown signal. Active games will be unmuted before the Discord session closes.")

	// 通常のDocker更新・再起動ではコマンドを削除しません。
	if os.Getenv("DELETE_COMMANDS_ON_SHUTDOWN") == "true" && !isOfficial && shardList.isPrimaryShard() {
		log.Println("Deleting slash commands")
		for _, registered := range registeredCommands {
			if registered.GuildID == "" {
				log.Printf("Deleting command %s GLOBALLY\n", registered.ApplicationCommand.Name)
			} else {
				log.Printf("Deleting command %s on guild %s\n", registered.ApplicationCommand.Name, registered.GuildID)
			}
			if err := bots[0].PrimarySession.ApplicationCommandDelete(
				registered.ApplicationCommand.ApplicationID,
				registered.GuildID,
				registered.ApplicationCommand.ID,
			); err != nil {
				log.Println(err)
			}
		}
		log.Println("Finished deleting all commands")
	}

	return nil
}

type shards []uint8

func defaultShard() shards {
	return []uint8{0}
}

// isPrimaryShard ensures that the first shard is shard 0.
func (sr shards) isPrimaryShard() bool {
	return len(sr) > 0 && sr[0] == 0
}

func parseShards(str string, maxShards int) (shards, error) {
	var result shards
	tokens := strings.Split(strings.ReplaceAll(str, " ", ""), ",")
	for _, token := range tokens {
		value, err := strconv.ParseUint(token, 10, 64)
		if err != nil {
			return result, err
		}
		if value >= uint64(maxShards) {
			return result, fmt.Errorf("shard: %d is greater or equal to the total max shards: %d", value, maxShards)
		}
		result = append(result, uint8(value))
	}
	return result, nil
}

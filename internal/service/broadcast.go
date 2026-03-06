package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"marketplace-bot/internal/database"
)

type BroadcastService struct {
	bot     *tgbotapi.BotAPI
	repo    *database.Repository
	running bool
	stopCh  chan struct{}
	mu      sync.Mutex
}

func NewBroadcastService(bot *tgbotapi.BotAPI, repo *database.Repository) *BroadcastService {
	return &BroadcastService{
		bot:  bot,
		repo: repo,
	}
}

func (s *BroadcastService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *BroadcastService) StartBroadcast(ctx context.Context, id int64) error {
	b, err := s.repo.GetBroadcastByID(ctx, id)
	if err != nil {
		return fmt.Errorf("рассылка #%d не найдена", id)
	}

	total, _ := s.repo.GetTotalUsersCount(ctx)
	_ = s.repo.SetBroadcastTotalUsers(ctx, id, total)
	_ = s.repo.UpdateBroadcastStatus(ctx, id, database.BroadcastRunning)

	s.mu.Lock()
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.run(b)
	return nil
}

func (s *BroadcastService) StopBroadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running && s.stopCh != nil {
		close(s.stopCh)
		s.running = false
	}
}

func (s *BroadcastService) ResumeBroadcast(ctx context.Context) error {
	broadcasts, err := s.repo.GetAllBroadcasts(ctx)
	if err != nil {
		return err
	}
	for _, b := range broadcasts {
		if b.Status == database.BroadcastPaused || b.Status == database.BroadcastRunning {
			_ = s.repo.UpdateBroadcastStatus(ctx, b.ID, database.BroadcastRunning)

			s.mu.Lock()
			s.running = true
			s.stopCh = make(chan struct{})
			s.mu.Unlock()

			bCopy := b
			go s.run(&bCopy)
			return nil
		}
	}
	return fmt.Errorf("нет приостановленных рассылок")
}

func (s *BroadcastService) run(b *database.Broadcast) {
	ctx := context.Background()
	lastID := b.LastUserID
	sent := b.SentCount
	failed := b.FailedCount

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	for {
		select {
		case <-s.stopCh:
			_ = s.repo.UpdateBroadcastStatus(ctx, b.ID, database.BroadcastPaused)
			_ = s.repo.UpdateBroadcastProgress(ctx, b.ID, sent, failed, lastID)
			return
		default:
		}

		users, err := s.repo.GetUsersForBroadcast(ctx, lastID, 50)
		if err != nil || len(users) == 0 {
			break
		}

		for _, u := range users {
			select {
			case <-s.stopCh:
				_ = s.repo.UpdateBroadcastStatus(ctx, b.ID, database.BroadcastPaused)
				_ = s.repo.UpdateBroadcastProgress(ctx, b.ID, sent, failed, lastID)
				return
			default:
			}

			m := tgbotapi.NewMessage(u.TelegramID, b.Text)
			m.ParseMode = "Markdown"

			if b.ButtonText != nil && b.ButtonURL != nil {
				m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonURL(*b.ButtonText, *b.ButtonURL),
					),
				)
			}

			if _, err := s.bot.Send(m); err != nil {
				failed++
			} else {
				sent++
			}

			lastID = u.ID
			time.Sleep(50 * time.Millisecond) // антиспам лимит Telegram
		}

		_ = s.repo.UpdateBroadcastProgress(ctx, b.ID, sent, failed, lastID)
	}

	_ = s.repo.UpdateBroadcastStatus(ctx, b.ID, database.BroadcastCompleted)
	_ = s.repo.UpdateBroadcastProgress(ctx, b.ID, sent, failed, lastID)
	log.Printf("Broadcast #%d completed: sent=%d failed=%d", b.ID, sent, failed)
}

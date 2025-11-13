#!/bin/bash
# run_with_gui.sh - Запуск Notepad_Go с виртуальным дисплеем

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🚀 Notepad_Go - Запуск с виртуальным дисплеем (Xvfb)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Проверка Xvfb
if ! command -v xvfb-run &> /dev/null; then
    echo "❌ Xvfb не установлен!"
    echo "Установите: sudo apt-get install -y xvfb"
    exit 1
fi

# Проверка наличия исходников
if [ ! -f "main.go" ]; then
    echo "❌ main.go не найден! Запустите из корня проекта"
    exit 1
fi

# Сборка
echo "🏗️  Сборка приложения..."
if ! go build -o notepad_go 2>&1; then
    echo "❌ Ошибка сборки!"
    exit 1
fi
echo "✅ Сборка успешна"
echo ""

# Запуск с Xvfb
echo "🖥️  Запуск с виртуальным дисплеем..."
echo "⚠️  ВНИМАНИЕ: Графическое окно НЕ появится (headless mode)"
echo "   Приложение запустится в фоне для тестирования"
echo ""
echo "📝 Логи приложения:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Запуск с логированием
FYNE_DEBUG=1 xvfb-run -a -s "-screen 0 1920x1080x24" ./notepad_go 2>&1 &
APP_PID=$!

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Приложение запущено (PID: $APP_PID)"
echo ""
echo "Для остановки: kill $APP_PID"
echo "Или нажмите Ctrl+C"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Ждем завершения
wait $APP_PID

#!/bin/bash
# run_with_vnc.sh - Запуск Notepad_Go с VNC (видимый GUI!)

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🖥️  Notepad_Go - Запуск с VNC (ВИДИМЫЙ GUI!)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Проверка зависимостей
check_dependency() {
    if ! command -v $1 &> /dev/null; then
        echo "❌ $1 не установлен!"
        echo "Установите: sudo apt-get install -y $2"
        exit 1
    fi
}

echo "🔍 Проверка зависимостей..."
check_dependency "Xvfb" "xvfb"
check_dependency "x11vnc" "x11vnc"

# Проверка наличия исходников
if [ ! -f "main.go" ]; then
    echo "❌ main.go не найден! Запустите из корня проекта"
    exit 1
fi

# Убиваем старые процессы
echo "🧹 Очистка старых процессов..."
pkill -f "Xvfb :99" 2>/dev/null || true
pkill -f "x11vnc.*:99" 2>/dev/null || true
sleep 1

# Запуск Xvfb
echo "🖥️  Запуск виртуального дисплея :99..."
Xvfb :99 -screen 0 1920x1080x24 &
XVFB_PID=$!
export DISPLAY=:99
sleep 2

# Проверка Xvfb
if ! ps -p $XVFB_PID > /dev/null; then
    echo "❌ Не удалось запустить Xvfb"
    exit 1
fi
echo "✅ Xvfb запущен (PID: $XVFB_PID)"

# Запуск x11vnc
echo "🔐 Запуск VNC сервера на порту 5900..."
x11vnc -display :99 -forever -shared -rfbport 5900 -nopw &
VNC_PID=$!
sleep 2

# Проверка VNC
if ! ps -p $VNC_PID > /dev/null; then
    echo "❌ Не удалось запустить VNC сервер"
    kill $XVFB_PID 2>/dev/null || true
    exit 1
fi
echo "✅ VNC сервер запущен (PID: $VNC_PID)"

# Сборка приложения
echo ""
echo "🏗️  Сборка приложения..."
if ! go build -o notepad_go 2>&1; then
    echo "❌ Ошибка сборки!"
    kill $XVFB_PID $VNC_PID 2>/dev/null || true
    exit 1
fi
echo "✅ Сборка успешна"

# Запуск приложения
echo ""
echo "🚀 Запуск Notepad_Go..."
./notepad_go &
APP_PID=$!
sleep 2

# Проверка приложения
if ! ps -p $APP_PID > /dev/null; then
    echo "❌ Приложение завершилось с ошибкой"
    kill $XVFB_PID $VNC_PID 2>/dev/null || true
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ ВСЕ ЗАПУЩЕНО УСПЕШНО!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📺 Для просмотра GUI подключитесь VNC клиентом:"
echo ""
echo "   Адрес:  localhost:5900"
echo "   Пароль: <не требуется>"
echo ""
echo "🔧 VNC клиенты:"
echo "   • Windows: TightVNC, UltraVNC, RealVNC"
echo "   • Linux:   vncviewer localhost:5900"
echo "   • macOS:   Screen Sharing (Finder -> Go -> Connect)"
echo ""
echo "💡 Порт проброшен? Проверьте:"
echo "   netstat -tlnp | grep 5900"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Процессы:"
echo "   Xvfb:        PID $XVFB_PID"
echo "   VNC Server:  PID $VNC_PID"
echo "   Notepad_Go:  PID $APP_PID"
echo ""
echo "Для остановки нажмите Ctrl+C"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Cleanup функция
cleanup() {
    echo ""
    echo "🛑 Останавливаем все процессы..."
    kill $APP_PID $VNC_PID $XVFB_PID 2>/dev/null || true
    echo "✅ Завершено"
    exit 0
}

trap cleanup SIGINT SIGTERM

# Ждем
while ps -p $APP_PID > /dev/null; do
    sleep 2
done

echo "❌ Приложение завершилось"
cleanup

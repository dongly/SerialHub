const { chromium } = require('playwright');

(async () => {
    console.log('=== UI 自动化测试 - xterm 终端 ===\n');
    
    // 使用系统 Chrome 浏览器
    const browser = await chromium.launch({ 
        headless: false,
        executablePath: 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
    });
    const context = await browser.newContext();
    const page = await context.newPage();
    
    try {
        // 测试 1: 打开终端页面
        console.log('测试 1: 打开终端页面...');
        await page.goto('http://127.0.0.1:5000/terminal');
        await page.waitForLoadState('networkidle');
        console.log('  ✅ 页面加载成功\n');
        
        // 测试 2: 检查控制面板元素
        console.log('测试 2: 检查控制面板元素...');
        
        const portSelect = await page.$('#port-select');
        console.log(portSelect ? '  ✅ 串口选择下拉框存在' : '  ❌ 串口选择下拉框不存在');
        
        const connectBtn = await page.$('#connect-btn');
        console.log(connectBtn ? '  ✅ 连接按钮存在' : '  ❌ 连接按钮不存在');
        
        const disconnectBtn = await page.$('#disconnect-btn');
        console.log(disconnectBtn ? '  ✅ 断开按钮存在' : '  ❌ 断开按钮不存在');
        
        const refreshBtn = await page.$('#refresh-btn');
        console.log(refreshBtn ? '  ✅ 刷新按钮存在' : '  ❌ 刷新按钮不存在');
        
        const terminalContainer = await page.$('#terminal-container');
        console.log(terminalContainer ? '  ✅ 终端容器存在' : '  ❌ 终端容器不存在');
        
        const statusIndicator = await page.$('#status-indicator');
        console.log(statusIndicator ? '  ✅ 状态指示灯存在' : '  ❌ 状态指示灯不存在');
        
        console.log('');
        
        // 测试 3: 检查按钮样式
        console.log('测试 3: 检查按钮样式...');
        if (connectBtn) {
            const connectClass = await connectBtn.getAttribute('class');
            console.log(connectClass === 'connect' ? '  ✅ 连接按钮样式类正确 (connect)' : `  ⚠️  连接按钮样式类: ${connectClass}`);
        }
        
        if (disconnectBtn) {
            const disconnectClass = await disconnectBtn.getAttribute('class');
            console.log(disconnectClass === 'disconnect' ? '  ✅ 断开按钮样式类正确 (disconnect)' : `  ⚠️  断开按钮样式类: ${disconnectClass}`);
        }
        
        if (refreshBtn) {
            const refreshClass = await refreshBtn.getAttribute('class');
            console.log(refreshClass === 'refresh' ? '  ✅ 刷新按钮样式类正确 (refresh)' : `  ⚠️  刷新按钮样式类: ${refreshClass}`);
        }
        
        console.log('');
        
        // 测试 4: 检查串口列表
        console.log('测试 4: 检查串口列表...');
        await page.waitForTimeout(2000); // 等待 WebSocket 连接和消息
        
        const options = await page.$$eval('#port-select option', opts => opts.map(o => o.textContent));
        console.log(`  下拉框选项: ${options.join(', ')}`);
        
        if (options.length > 1) {
            console.log('  ✅ 串口列表已加载');
        } else {
            console.log('  ⚠️  串口列表为空或正在加载');
        }
        
        console.log('');
        
        // 测试 5: 检查终端内容
        console.log('测试 5: 检查终端内容...');
        const terminalText = await page.$eval('#terminal-container', el => el.textContent);
        
        if (terminalText.includes('[SerialHub] Terminal Connected')) {
            console.log('  ✅ 终端显示欢迎消息');
        }
        
        if (terminalText.includes('COM7')) {
            console.log('  ✅ 终端显示串口信息');
        }
        
        // 检查是否没有 JSON 消息显示在终端中
        if (!terminalText.includes('"type":"ports"')) {
            console.log('  ✅ JSON 消息未显示在终端中');
        } else {
            console.log('  ❌ JSON 消息错误地显示在终端中');
        }
        
        console.log('');
        
        // 测试 6: 测试刷新按钮
        console.log('测试 6: 测试刷新按钮...');
        if (refreshBtn) {
            await refreshBtn.click();
            await page.waitForTimeout(1000);
            console.log('  ✅ 刷新按钮可点击');
        }
        
        console.log('');
        
        // 测试 7: 截图保存
        console.log('测试 7: 保存截图...');
        await page.screenshot({ path: 'xterm-ui-test.png', fullPage: true });
        console.log('  ✅ 截图已保存: xterm-ui-test.png\n');
        
        console.log('=== UI 自动化测试完成 ===');
        
    } catch (error) {
        console.error('测试错误:', error);
    } finally {
        await browser.close();
    }
})();
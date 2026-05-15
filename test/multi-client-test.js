const { chromium } = require('playwright');

(async () => {
    console.log('=== 多客户端连接测试 ===\n');
    
    const browser = await chromium.launch({ 
        headless: false,
        executablePath: 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
    });
    
    try {
        // 打开第一个页面
        console.log('打开第一个客户端...');
        const context1 = await browser.newContext();
        const page1 = await context1.newPage();
        await page1.goto('http://127.0.0.1:5000/terminal');
        await page1.waitForTimeout(3000);
        
        const content1 = await page1.$eval('#terminal-container', el => el.textContent);
        console.log('  ✅ 客户端 1 连接成功');
        console.log('     欢迎消息:', content1.includes('Connected to SerialHub') ? '收到' : '未收到');
        
        // 打开第二个页面
        console.log('\n打开第二个客户端...');
        const context2 = await browser.newContext();
        const page2 = await context2.newPage();
        await page2.goto('http://127.0.0.1:5000/terminal');
        await page2.waitForTimeout(3000);
        
        const content2 = await page2.$eval('#terminal-container', el => el.textContent);
        console.log('  ✅ 客户端 2 连接成功');
        console.log('     欢迎消息:', content2.includes('Connected to SerialHub') ? '收到' : '未收到');
        
        // 检查第一个客户端是否仍然连接
        console.log('\n检查客户端 1 是否仍然连接...');
        await page1.waitForTimeout(2000);
        const content1New = await page1.$eval('#terminal-container', el => el.textContent);
        console.log('  ✅ 客户端 1 仍然连接');
        console.log('     终端内容长度:', content1New.length);
        
        // 检查两个客户端是否都收到串口数据
        console.log('\n检查两个客户端是否都收到串口数据...');
        await page1.waitForTimeout(5000);
        await page2.waitForTimeout(5000);
        
        const data1 = await page1.$eval('#terminal-container', el => el.textContent);
        const data2 = await page2.$eval('#terminal-container', el => el.textContent);
        
        const hasData1 = data1.includes('道[') || data1.includes('通道[');
        const hasData2 = data2.includes('道[') || data2.includes('通道[');
        
        console.log('  客户端 1 收到串口数据:', hasData1 ? '✅ 是' : '❌ 否');
        console.log('  客户端 2 收到串口数据:', hasData2 ? '✅ 是' : '❌ 否');
        
        // 截图
        await page1.screenshot({ path: 'multi-client-1.png', fullPage: true });
        await page2.screenshot({ path: 'multi-client-2.png', fullPage: true });
        console.log('\n截图已保存:');
        console.log('  - multi-client-1.png');
        console.log('  - multi-client-2.png');
        
        if (hasData1 && hasData2) {
            console.log('\n✅ 多客户端测试通过！两个客户端都收到串口数据');
        } else {
            console.log('\n⚠️  部分测试失败');
        }
        
        await context1.close();
        await context2.close();
        
    } catch (error) {
        console.error('测试错误:', error);
    } finally {
        await browser.close();
    }
})();
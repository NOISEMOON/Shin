// 从 cookie 中获取指定的 cookie 值
function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) return parts.pop().split(';').shift();
    return null;
}

// 获取 token 值
function getToken() {
    return getCookie("token");
}